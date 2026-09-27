package workflow

// The inbound gate this column's session passes every line through, played as
// Claude Code 2.1.280 plays it (docs/research-launch.md, the cross-session
// inbound gate; docs/research.md for what was seen live).
//
// A line that arrives before the session is up is held and released once it
// is. After that the session accepts, or — as a case asks — holds a line and
// later releases it, holds it until it expires, refuses it, or says it holds it
// only after rewake has stopped waiting for a first word, and then lets it
// expire. Everything but a
// plain accept is reported to the reply socket the line names, on a new
// connection, as one receipt line quoting the line's msg_id.
//
// Stricter than the harness where it can be, never looser. The real one sends
// receipts only when asked and checks the reply socket first; this one also
// refuses a line that does not ask, since rewake always should, and refuses a
// msg_id that is not a UUID, which the real one would merely not quote back.
// It holds at startup for longer than the real one does — half a second rather
// than two tenths — so a first notice that does not wait for the session is
// caught every time rather than most times. And it can be slower to speak than
// the real one was seen to be: live, a hold was reported within 30 ms, but
// nothing bounds that on a loaded machine, so "hold-late" reports its hold well
// after rewake's window for a first word has closed.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

const (
	// shimInbound makes the session hold or refuse what it receives once it
	// is up: "hold-release", "hold-expire", "hold-late" or "refuse". Unset,
	// it accepts.
	shimInbound = "RW_SHIM_INBOUND"
	// shimInboundFile is where the session records what its gate did.
	shimInboundFile = "RW_SHIM_INBOUND_FILE"
)

// The session's startup, measured from the moment its socket listens. Live,
// SessionStart ran about 80 ms in, the gate stopped holding about 100 ms later,
// and the status line ran after that (docs/research.md).
const (
	sessionStartAfter = 100 * time.Millisecond
	mountedAfter      = 600 * time.Millisecond
	// releaseAfter is how long "hold-release" holds; less than send's own
	// wait, so the sender sees the release. expireAfter is longer than that
	// wait, so the sender sees the hold.
	releaseAfter = 1500 * time.Millisecond
	expireAfter  = 7 * time.Second
	// workAfterRelease is how long a released line waits before its turn.
	workAfterRelease = time.Second
	// lateHoldAfter is when "hold-late" reports its hold: well past the
	// 300 ms rewake waits for a first word. lateExpireAfter is how long that
	// hold lasts before it expires.
	lateHoldAfter   = 900 * time.Millisecond
	lateExpireAfter = 2 * time.Second
)

// heldReason is the one sentence the real gate puts in every held receipt,
// whatever held the line.
const heldReason = "Your message is held for the recipient user's approval before it reaches their Claude session (permission-mode parity)."

var uuidShape = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// inboundLine is what the gate needs of one delivery: where to report, what
// to quote, and who wrote it.
type inboundLine struct {
	from   string
	msgID  string
	writer peer
}

// peer is a process at the other end of a unix socket.
type peer struct {
	pid, uid int
	start    string
}

// mount plays the session coming up: SessionStart, then the moment the gate
// stops holding, then the first status line. What was held in between is
// released in order. Without hooks only the gate opens: the telemetry script
// plays the hooks and the status line itself.
func (s *claudeSession) mount(hooks bool) {
	time.Sleep(sessionStartAfter)
	if payload, err := json.Marshal(map[string]any{
		"hook_event_name": "SessionStart", "session_id": shimConversation(),
		"cwd": workingDirectory(), "source": s.startSource(),
	}); err == nil && hooks {
		s.runHook("SessionStart", s.launch.settings.observe, payload)
		s.plugin.event("session.start", map[string]any{"cwd": workingDirectory(), "isInteractive": true, "surface": "terminal"})
	}
	time.Sleep(time.Until(s.listening.Add(mountedAfter)))
	s.mu.Lock()
	s.mounted = true
	held := s.startupHeld
	s.startupHeld = nil
	s.mu.Unlock()
	if mark := os.Getenv(shimReadyFile); mark != "" {
		_ = os.WriteFile(mark+".mounted", nil, 0o600)
	}
	for _, release := range held {
		s.receipt(release.line, "delivered", "")
		s.recordInbound("released", release.line.msgID)
		s.workTurn(release.turn, release.notice)
	}
	if !hooks {
		return
	}
	status := exec.Command("/bin/sh", "-c", s.launch.settings.statusLine)
	status.Env = os.Environ()
	status.Stdin = strings.NewReader(string(statusPayload(-1, shimConversation())))
	_ = status.Run()
}

// startupRelease is a line held at startup, with what its turn needs.
type startupRelease struct {
	line   inboundLine
	turn   string
	notice claudeNotice
}

// gate decides what happens to a checked line, and reports whether the session
// works a turn on it now.
func (s *claudeSession) gate(line inboundLine, turn string, notice claudeNotice) bool {
	s.mu.Lock()
	if !s.mounted {
		s.startupHeld = append(s.startupHeld, startupRelease{line: line, turn: turn, notice: notice})
		s.mu.Unlock()
		s.recordInbound("held-at-startup", line.msgID)
		s.receipt(line, "held", "")
		return false
	}
	s.mu.Unlock()
	switch os.Getenv(shimInbound) {
	case "refuse":
		s.recordInbound("refused", line.msgID)
		s.receipt(line, "expired", "refused")
		return false
	case "hold-release":
		s.recordInbound("held", line.msgID)
		s.receipt(line, "held", "")
		go func() {
			time.Sleep(releaseAfter)
			s.receipt(line, "delivered", "")
			s.recordInbound("released", line.msgID)
			// A released line joins the queue and waits for the agent, so
			// the sender, polling, sees it delivered before it is read.
			time.Sleep(workAfterRelease)
			s.workTurn(turn, notice)
		}()
		return false
	case "hold-expire":
		s.recordInbound("held", line.msgID)
		s.receipt(line, "held", "")
		go func() {
			time.Sleep(expireAfter)
			s.receipt(line, "expired", "")
			s.recordInbound("expired", line.msgID)
		}()
		return false
	case "hold-late":
		go func() {
			time.Sleep(lateHoldAfter)
			s.recordInbound("held", line.msgID)
			s.receipt(line, "held", "")
			time.Sleep(lateExpireAfter)
			s.receipt(line, "expired", "")
			s.recordInbound("expired", line.msgID)
		}()
		return false
	}
	s.recordInbound("accepted", line.msgID)
	return true
}

// checkedReply reads the reply address and id of a line, refusing a line that
// does not ask for receipts in the one form the receiver answers.
func checkedReply(raw []byte, socket string, writer peer) (inboundLine, error) {
	var fields struct {
		From  string `json:"from"`
		MsgID string `json:"msg_id"`
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return inboundLine{}, err
	}
	path, found := strings.CutPrefix(fields.From, "uds:")
	if !found || !strings.HasSuffix(path, ".sock") || filepath.Dir(path) != filepath.Dir(socket) {
		return inboundLine{}, fmt.Errorf("a reply address the session does not answer: %q", fields.From)
	}
	if !uuidShape.MatchString(fields.MsgID) {
		return inboundLine{}, fmt.Errorf("a msg_id that is not a UUID: %q", fields.MsgID)
	}
	return inboundLine{from: path, msgID: fields.MsgID, writer: writer}, nil
}

// receipt reports one step to the line's reply socket, on a new connection,
// after checking that the listener is the process that wrote the line. A check
// that fails is recorded, and no receipt goes.
func (s *claudeSession) receipt(line inboundLine, status, detail string) {
	if err := s.sendReceipt(line, status, detail); err != nil {
		s.recordInbound("receipt-refused", line.msgID+" "+err.Error())
	}
}

func (s *claudeSession) sendReceipt(line inboundLine, status, detail string) error {
	connection, err := net.DialTimeout("unix", line.from, 2*time.Second)
	if err != nil {
		return err
	}
	defer func() { _ = connection.Close() }()
	listener, err := peerOf(connection)
	if err != nil {
		return err
	}
	if listener != line.writer {
		return fmt.Errorf("the reply socket is listened on by %+v, the line was written by %+v", listener, line.writer)
	}
	reason := heldReason
	if status != "held" {
		reason = "The message was " + status + "."
	}
	receipt := map[string]any{
		"type": "control", "action": "peer_message_status", "status": status, "reason": reason,
		"from": "uds:" + s.launch.socket, "orig_msg_id": line.msgID, "msgV": 1, "msg_id": newShimUUID(),
	}
	if detail != "" {
		receipt["status_detail"] = detail
	}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	_ = connection.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, err = connection.Write(append(encoded, '\n'))
	return err
}

// peerOf names the process at the other end of a unix connection.
func peerOf(connection net.Conn) (peer, error) {
	unix, ok := connection.(*net.UnixConn)
	if !ok {
		return peer{}, errors.New("not a unix connection")
	}
	raw, err := unix.SyscallConn()
	if err != nil {
		return peer{}, err
	}
	var credentials *syscall.Ucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		credentials, credErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return peer{}, err
	}
	if credErr != nil {
		return peer{}, credErr
	}
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", credentials.Pid))
	if err != nil {
		return peer{}, err
	}
	// The start time is field 22; the command name before it may hold spaces
	// and ends at the last ')'.
	after := string(stat[strings.LastIndexByte(string(stat), ')')+1:])
	fields := strings.Fields(after)
	if len(fields) < 20 {
		return peer{}, errors.New("an unreadable process record")
	}
	return peer{pid: int(credentials.Pid), uid: int(credentials.Uid), start: fields[19]}, nil
}

func (s *claudeSession) recordInbound(event, detail string) {
	if target := os.Getenv(shimInboundFile); target != "" {
		appendLine(target, event+"\t"+detail)
	}
}

func newShimUUID() string {
	var b [16]byte
	if f, err := os.Open("/dev/urandom"); err == nil {
		_, _ = f.Read(b[:])
		_ = f.Close()
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
