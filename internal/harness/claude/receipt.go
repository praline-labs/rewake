package claude

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/inbox"
)

// receiptLine is a receipt as Claude Code writes it, read on 2.1.280 from the
// bundled source and seen live (docs/research-launch.md):
//
//	{"type":"control","action":"peer_message_status","status":"held",
//	 "reason":"…","from":"uds:…","orig_msg_id":"…","msgV":1,"msg_id":"…"}
//
// A refusal is "expired" with status_detail "refused"; a drop names the lines
// it dropped in dropped_msg_ids.
type receiptLine struct {
	Type         string   `json:"type"`
	Action       string   `json:"action"`
	Status       string   `json:"status"`
	StatusDetail string   `json:"status_detail"`
	Reason       string   `json:"reason"`
	DropReason   string   `json:"drop_reason"`
	OrigMsgID    string   `json:"orig_msg_id"`
	DroppedIDs   []string `json:"dropped_msg_ids"`
}

// word is what one receipt says about one line rewake wrote.
type word struct {
	msgID  string
	result inbox.Result
}

// parseReceipt reads one line. Anything that is not a receipt, or names a
// status this adapter does not know, says nothing: an unknown status could mean
// either outcome, and guessing would tell a sender the wrong one.
func parseReceipt(raw []byte) []word {
	var line receiptLine
	if json.Unmarshal(raw, &line) != nil || line.Type != "control" || line.Action != "peer_message_status" {
		return nil
	}
	result, ok := receiptResult(line)
	if !ok {
		return nil
	}
	ids := line.DroppedIDs
	if line.OrigMsgID != "" {
		ids = append([]string{line.OrigMsgID}, ids...)
	}
	var words []word
	seen := map[string]bool{}
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			words = append(words, word{msgID: id, result: result})
		}
	}
	return words
}

// receiptResult turns a status into what the sender is told. The reason is the
// harness's own sentence and is passed on as it came.
func receiptResult(line receiptLine) (inbox.Result, bool) {
	because := func(what, reason string) string {
		if reason = strings.TrimSpace(reason); reason != "" {
			return what + ": " + reason
		}
		return what
	}
	switch line.Status {
	case "held":
		return inbox.Result{State: inbox.Held, Via: "socket", Detail: because("the session holds the notice and has not shown it to the agent", line.Reason)}, true
	case "delivered":
		return inbox.Result{State: inbox.Delivered, Via: "socket", Detail: "released after being held"}, true
	case "expired":
		if line.StatusDetail == "refused" {
			return inbox.Result{State: inbox.Failed, Via: "socket", Detail: because("the session refuses messages from other sessions", line.Reason)}, true
		}
		return inbox.Result{State: inbox.Failed, Via: "socket", Detail: because("the session held the notice and it expired unreleased", line.Reason)}, true
	case "denied":
		return inbox.Result{State: inbox.Failed, Via: "socket", Detail: because("the person at the session declined the notice", line.Reason)}, true
	case "dropped":
		return inbox.Result{State: inbox.Failed, Via: "socket", Detail: because("the session dropped the notice", line.DropReason)}, true
	}
	return inbox.Result{}, false
}

// sameUser reports whether the connected peer runs as this user.
func sameUser(connection interface {
	SyscallConn() (syscall.RawConn, error)
},
) bool {
	raw, err := connection.SyscallConn()
	if err != nil {
		return false
	}
	var credentials *syscall.Ucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		credentials, credErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil || credErr != nil {
		return false
	}
	return int(credentials.Uid) == os.Getuid()
}

// maxReceipt bounds one receipt line. A receipt is a handful of short fields.
const maxReceipt = 64 << 10

// acceptPause bounds the wait after a failed accept — out of descriptors, say —
// which would otherwise be retried at once, and again, on a whole core.
const acceptPause = time.Second

func (l *lane) accept(listener *net.UnixListener) {
	var pause time.Duration
	for {
		connection, err := listener.AcceptUnix()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			pause = min(max(2*pause, 5*time.Millisecond), acceptPause)
			select {
			case <-l.done:
				return
			case <-time.After(pause):
			}
			continue
		}
		pause = 0
		go l.read(connection)
	}
}

// read takes the receipts one connection carries. Only this user's processes
// may write here — the directory says so, and the peer is checked for a
// directory somebody loosened.
func (l *lane) read(connection *net.UnixConn) {
	defer func() { _ = connection.Close() }()
	if !sameUser(connection) {
		return
	}
	_ = connection.SetReadDeadline(time.Now().Add(dialTimeout))
	scanner := bufio.NewScanner(connection)
	scanner.Buffer(make([]byte, 0, 4096), maxReceipt)
	for scanner.Scan() {
		for _, word := range parseReceipt(scanner.Bytes()) {
			l.route(word.msgID, word.result)
		}
	}
}
