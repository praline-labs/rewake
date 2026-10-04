package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
)

// The socket path of delivery: the notice written to the session's messaging
// socket when no lane is listening, and the socket file's own upkeep.

// maxSocketPath is the limit the kernel puts on a unix socket path.
const maxSocketPath = 103

// Deliver writes the notice with no reply address, so the session reports
// nothing back and a successful write is all there is to know. The lane
// (lane.go) is the path a running wrapper uses; this one is what is left when
// it cannot listen.
func (claudeHarness) Deliver(ctx context.Context, session registry.Session, message inbox.Message) inbox.Result {
	return writeNotice(ctx, session, newEnvelope(message, ""))
}

func newEnvelope(message inbox.Message, interrupter string) envelope {
	return envelope{
		Type:     "user",
		Message:  payload{Role: "user", Content: notification(message, interrupter)},
		Priority: "next",
	}
}

// writeNotice writes one line to the session's inbox socket.
func writeNotice(ctx context.Context, session registry.Session, notice envelope) inbox.Result {
	if session.Socket == "" {
		return inbox.Result{State: inbox.Failed, Detail: "this session has no inbox socket"}
	}

	line, err := json.Marshal(notice)
	if err != nil {
		return inbox.Result{State: inbox.Failed, Detail: "the message could not be encoded: " + err.Error()}
	}
	dialer := net.Dialer{Timeout: dialTimeout}
	connection, err := dialer.DialContext(ctx, "unix", session.Socket)
	if err != nil {
		// The socket appears a moment after the process does, and it is
		// recreated across a restart, so a session that is still alive gets
		// another attempt rather than a refusal.
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ECONNREFUSED) {
			return inbox.Result{State: inbox.Pending, Detail: "the session is not listening yet"}
		}
		return inbox.Result{State: inbox.Failed, Detail: "could not reach the session: " + err.Error()}
	}
	defer func() { _ = connection.Close() }()

	_ = connection.SetWriteDeadline(time.Now().Add(dialTimeout))
	if _, err := connection.Write(append(line, '\n')); err != nil {
		return inbox.Result{State: inbox.Failed, Detail: "could not write to the session: " + err.Error()}
	}
	return inbox.Result{State: inbox.Delivered, Via: "socket"}
}

// envelope is one line of the session inbox protocol. Priority "next" puts the
// message after the tool call in flight and starts a turn when the session is
// idle, which is what a message from a peer should do.
//
// From and MsgID ask for receipts: the session reports what its inbound gate
// did with the line to the socket From names, quoting MsgID (lane.go).
type envelope struct {
	Type     string  `json:"type"`
	Message  payload `json:"message"`
	Priority string  `json:"priority"`
	From     string  `json:"from,omitempty"`
	MsgID    string  `json:"msg_id,omitempty"`
}

type payload struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// removeStaleSocket clears a socket file left by a session that is gone.
//
// Only one error proves the owner is gone: connection refused, which the kernel
// returns when a socket file has no listener behind it. Every other failure —
// a full backlog, a timeout, a permission error — describes this moment, not the
// owner, and deleting the file then cuts off a session that is still running.
func removeStaleSocket(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return nil
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("%s exists and is not a socket; remove it or use another session name", path)
	}

	connection, err := net.DialTimeout("unix", path, 200*time.Millisecond)
	if err == nil {
		_ = connection.Close()
		return fmt.Errorf("%s is a live socket; another session is using this name", path)
	}
	if !errors.Is(err, syscall.ECONNREFUSED) {
		return fmt.Errorf("%s exists and could not be checked (%v); leaving it alone", path, err)
	}
	return os.Remove(path)
}
