package endpoint

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/tool"
)

// The clients of the endpoint: a child's one confirmation, and a transport's
// one call. Each checks that the listener is its own run's wrapper — this
// user, and a process it runs below — before it says anything.

// ErrUnreachable says the endpoint did not answer: no wrapper, a wrapper that
// is gone, or a listener that is not this run's wrapper.
var ErrUnreachable = errors.New("the wrapper's context endpoint does not answer")

// ConfirmWait bounds Confirm, a ticket's short exchange with the wrapper
// (docs/mail-bridge-turns.md#waits-and-their-bounds).
const ConfirmWait = time.Second

// Confirm has the run's wrapper confirm a ticket for this process, once.
func Confirm(path string, ticket tool.Ticket) error {
	conn, err := greet(path, hello{Role: roleChild, Capability: ticket.Capability}, ConfirmWait)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	return ask(conn, request{ID: 1, Op: opConfirm, Ticket: &ticket})
}

// CallTool asks the run's wrapper to run one call for a transport, and
// waits at most limit for its answer. The caller must be the transport's
// process itself: the wrapper refuses any other, a child of it among them.
func CallTool(path string, call ToolCall, limit time.Duration) (ToolAnswer, error) {
	conn, err := greet(path, hello{Role: roleTransport}, shortExchange)
	if err != nil {
		return ToolAnswer{}, err
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(limit))
	encoded, err := json.Marshal(request{ID: 1, Op: opCall, Call: &call})
	if err != nil {
		return ToolAnswer{}, err
	}
	if _, err := conn.Write(append(encoded, '\n')); err != nil {
		return ToolAnswer{}, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	line, err := readLine(bufio.NewReader(conn))
	if err != nil {
		return ToolAnswer{}, fmt.Errorf("%w: no answer: %v", ErrUnreachable, err)
	}
	var answer response
	if err := json.Unmarshal(line, &answer); err != nil {
		return ToolAnswer{}, errors.New("an answer that does not parse")
	}
	if answer.Error != "" {
		return ToolAnswer{}, errors.New(answer.Error)
	}
	if answer.Answer == nil {
		return ToolAnswer{}, errors.New("the wrapper answered with no result")
	}
	return *answer.Answer, nil
}

// greet dials, checks the listener, and says hello; the deadline set here
// bounds the whole exchange.
func greet(path string, greeting hello, limit time.Duration) (*net.UnixConn, error) {
	raw, err := net.DialTimeout("unix", path, limit)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	conn := raw.(*net.UnixConn)
	_ = conn.SetDeadline(time.Now().Add(limit))
	if err := servesCaller(conn); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := ask(conn, greeting); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

// ask writes one line and reads one answer.
func ask(conn *net.UnixConn, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := conn.Write(append(encoded, '\n')); err != nil {
		return fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	line, err := readLine(bufio.NewReader(conn))
	if err != nil {
		return fmt.Errorf("%w: no answer: %v", ErrUnreachable, err)
	}
	var answer response
	if err := json.Unmarshal(line, &answer); err != nil {
		return errors.New("an answer that does not parse")
	}
	if answer.Error != "" {
		return errors.New(answer.Error)
	}
	return nil
}

// servesCaller checks that the listener is a process above the caller: its
// run's wrapper. A listener that bound the path first would otherwise take
// the call's words.
func servesCaller(conn *net.UnixConn) error {
	peer, err := peerOf(conn)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	if int(peer.Uid) != os.Getuid() {
		return fmt.Errorf("%w: the socket is served by another user", ErrUnreachable)
	}
	if err := proc.Default.Descends(os.Getpid(), int(peer.Pid)); err != nil {
		return fmt.Errorf("%w: the socket is served by process %d, which this process does not run below", ErrUnreachable, peer.Pid)
	}
	return nil
}

// The deadline of a ticket (docs/archive-1.x/mail-bridge-server.md#the-ticket).
const (
	span      = 25 * time.Second
	minSpan   = 5 * time.Second
	timeoutLe = 2 * time.Second
)

// DeadlineFor is how long a ticket of a transport lasts, and why the tool
// reads nothing when that is too short. variable names the setting of the
// transport's own timeout, in milliseconds, read from the wrapper's
// environment at launch; "" for a transport without one.
func DeadlineFor(variable string, getenv func(string) string) (time.Duration, string) {
	if variable == "" {
		return span, ""
	}
	setting := getenv(variable)
	millis, err := strconv.ParseInt(setting, 10, 64)
	if setting == "" || err != nil {
		return span, ""
	}
	limit := min(span, time.Duration(millis)*time.Millisecond-timeoutLe)
	if limit < minSpan {
		return limit, variable + "=" + setting + " leaves under five seconds for a tool call, so the tool runs nothing; raise it, or use the same words in the shell"
	}
	return limit, ""
}
