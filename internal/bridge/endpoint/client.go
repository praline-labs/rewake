package endpoint

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/proc"
)

// The clients of the endpoint: the server's connection, which lives as long
// as the server; the child's one confirmation; the hook's one observation.
// Each checks that the listener is its own run's wrapper — this user, and a
// process it runs below — before it says anything.

// ErrUnreachable says the endpoint did not answer: no wrapper, a wrapper that
// is gone, or a listener that is not this run's wrapper.
var ErrUnreachable = errors.New("the wrapper's context endpoint does not answer")

// The bounds of the short exchanges (docs/mail-bridge-turns.md#waits-and-their-bounds).
const (
	ConfirmWait = time.Second
	HookWait    = 2 * time.Second
)

// Client is a server's connection to its run's wrapper.
type Client struct {
	conn    *net.UnixConn
	writeMu sync.Mutex
	mu      sync.Mutex
	next    uint64
	waiting map[uint64]chan response
	failed  error
	gone    chan struct{}
}

// Dial connects a server, says hello, and starts reading answers.
func Dial(path, capability string) (*Client, error) {
	conn, err := greet(path, hello{Role: roleServer, Capability: capability}, shortExchange)
	if err != nil {
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	client := &Client{conn: conn, waiting: map[uint64]chan response{}, gone: make(chan struct{})}
	go client.read()
	return client, nil
}

// Gone is closed when the wrapper ends the connection.
func (c *Client) Gone() <-chan struct{} { return c.gone }

// Close ends the connection.
func (c *Client) Close() { _ = c.conn.Close() }

func (c *Client) read() {
	reader := bufio.NewReaderSize(c.conn, 64<<10)
	for {
		line, err := readLine(reader)
		var answer response
		if err == nil && json.Unmarshal(line, &answer) != nil {
			err = errors.New("an answer that does not parse")
		}
		if err != nil {
			c.mu.Lock()
			c.failed = fmt.Errorf("%w: %v", ErrUnreachable, err)
			for _, waiter := range c.waiting {
				close(waiter)
			}
			c.waiting = nil
			c.mu.Unlock()
			close(c.gone)
			return
		}
		c.mu.Lock()
		if waiter, ok := c.waiting[answer.ID]; ok {
			waiter <- answer
			delete(c.waiting, answer.ID)
		}
		c.mu.Unlock()
	}
}

// Ticket asks for the ticket of one call, waiting at most limit.
func (c *Client) Ticket(asked TicketRequest, limit time.Duration) (bridge.Ticket, error) {
	c.mu.Lock()
	if c.failed != nil {
		defer c.mu.Unlock()
		return bridge.Ticket{}, c.failed
	}
	c.next++
	id := c.next
	waiter := make(chan response, 1)
	c.waiting[id] = waiter
	c.mu.Unlock()
	encoded, _ := json.Marshal(request{ID: id, Op: opTicket, Ask: &asked})
	c.writeMu.Lock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(limit))
	_, err := c.conn.Write(append(encoded, '\n'))
	c.writeMu.Unlock()
	if err != nil {
		return bridge.Ticket{}, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	timer := time.NewTimer(limit)
	defer timer.Stop()
	select {
	case answer, ok := <-waiter:
		if !ok {
			return bridge.Ticket{}, ErrUnreachable
		}
		if answer.Error != "" {
			return bridge.Ticket{}, errors.New(answer.Error)
		}
		if answer.Ticket == nil {
			return bridge.Ticket{}, errors.New("the wrapper answered with no ticket")
		}
		return *answer.Ticket, nil
	case <-timer.C:
		c.mu.Lock()
		delete(c.waiting, id)
		c.mu.Unlock()
		return bridge.Ticket{}, errors.New("the wrapper did not answer in time")
	}
}

// Confirm has the run's wrapper confirm a ticket for this process, once.
func Confirm(path string, ticket bridge.Ticket) error {
	conn, err := greet(path, hello{Role: roleChild, Capability: ticket.Capability}, ConfirmWait)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	return ask(conn, request{ID: 1, Op: opConfirm, Ticket: &ticket})
}

// Observe hands a hook's input to the run's wrapper and returns once it
// recorded it, or the bound ran out.
func Observe(path string, payload []byte, limits HookLimits, limit time.Duration) error {
	if !json.Valid(payload) {
		return errors.New("the hook input is not JSON")
	}
	conn, err := greet(path, hello{Role: roleHook}, limit)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	return ask(conn, request{ID: 1, Op: opObserve, Payload: payload, Limits: &limits})
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

// The deadline of a ticket (docs/mail-bridge-server.md#the-ticket).
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
