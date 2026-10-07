//go:build rewakefixture

package fixture

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// The exchange between the adapter and its program: one JSON object per line
// over the unix socket the adapter listens on. Either side may ask; an
// answer carries the id of the request it answers, and each side numbers its
// own requests, so an answer always belongs to the side that receives it.

// Operations. The program says hello, answers, and reports its turns and
// activity; the adapter probes, reserves, delivers and releases.
const (
	opHello       = "hello"
	opAnswer      = "answer"
	opTurnStarted = "turn-started"
	opTurnEnded   = "turn-ended"
	opActivity    = "activity"
	opProbe       = "probe"
	opReserve     = "reserve"
	opDeliver     = "deliver"
	opRelease     = "release"
)

// The capabilities a program may serve, as its hello names them.
const (
	Wake         = "wake"
	TurnBoundary = "turn-boundary"
	Telemetry    = "telemetry"
	Control      = "control"
)

// Served is every capability a program can serve, in the order a hello
// lists them.
var Served = []string{Wake, TurnBoundary, Telemetry, Control}

// The outcomes a turn's end reports.
const (
	OutcomeCompleted   = "completed"
	OutcomeFailed      = "failed"
	OutcomeInterrupted = "interrupted"
)

// Frame is one line of the exchange. Every field but Op belongs to some
// operations only.
type Frame struct {
	Op string `json:"op"`
	ID int64  `json:"id,omitempty"`

	// A hello: the program's version, its pid, its conversation and what it
	// serves.
	Version string   `json:"version,omitempty"`
	PID     int      `json:"pid,omitempty"`
	Thread  string   `json:"thread,omitempty"`
	Serves  []string `json:"serves,omitempty"`

	// A probe names its capability; an answer says whether it went, and why
	// not. State is a probe's answer or the activity reported.
	Capability string `json:"capability,omitempty"`
	OK         bool   `json:"ok,omitempty"`
	Error      string `json:"error,omitempty"`
	State      string `json:"state,omitempty"`

	// A turn's start and end. End names the end's event: the same end sent
	// again carries the same name. Hold says the program can continue the
	// turn if the end is held; Reason is the core's answer when it is.
	Turn    string `json:"turn,omitempty"`
	End     string `json:"end,omitempty"`
	Outcome string `json:"outcome,omitempty"`
	Text    string `json:"text,omitempty"`
	Hold    bool   `json:"hold,omitempty"`
	Reason  string `json:"reason,omitempty"`

	// A delivery: its own id, the notice the session is shown and the
	// letters it carries.
	Letter  string   `json:"letter,omitempty"`
	Notice  string   `json:"notice,omitempty"`
	Members []Member `json:"members,omitempty"`
	Steered bool     `json:"steered,omitempty"`
}

// Member is one letter a delivery carries, by its identities.
type Member struct {
	ID        string `json:"id"`
	From      string `json:"from,omitempty"`
	FromEpoch string `json:"fromEpoch,omitempty"`
	To        string `json:"to,omitempty"`
	ToEpoch   string `json:"toEpoch,omitempty"`
	Recalls   string `json:"recalls,omitempty"`
	Replaces  string `json:"replaces,omitempty"`
}

// maxFrame bounds one line: a notice, its members and a turn's text.
const maxFrame = 4 << 20

// errClosed is a link whose connection is gone.
var errClosed = errors.New("the fixture's connection closed")

// link is one connection: a reader that hands answers to whoever waits for
// them and everything else to a handler, and a writer shared by both sides.
type link struct {
	conn net.Conn

	writeMu sync.Mutex

	mu      sync.Mutex
	next    int64
	waiting map[int64]chan Frame
	closed  chan struct{}
	once    sync.Once
}

func newLink(conn net.Conn) *link {
	return &link{conn: conn, waiting: map[int64]chan Frame{}, closed: make(chan struct{})}
}

// read returns the next frame, answers included; serve is what uses it after
// the hello.
func (l *link) read(reader *bufio.Reader) (Frame, error) {
	line, err := readLine(reader)
	if err != nil {
		return Frame{}, err
	}
	var frame Frame
	if err := json.Unmarshal(line, &frame); err != nil {
		return Frame{}, fmt.Errorf("a line that is not a frame: %w", err)
	}
	return frame, nil
}

// serve reads until the connection ends, routing answers to their waiters and
// every other frame to handle, on this goroutine: what a frame fixes about its
// moment — a turn's boundary — is fixed before the next line is read, and
// handle starts whatever waits on its own goroutine.
func (l *link) serve(reader *bufio.Reader, handle func(Frame)) {
	defer l.close()
	for {
		frame, err := l.read(reader)
		if err != nil {
			return
		}
		if frame.Op == opAnswer {
			l.mu.Lock()
			waiter := l.waiting[frame.ID]
			delete(l.waiting, frame.ID)
			l.mu.Unlock()
			if waiter != nil {
				waiter <- frame
			}
			continue
		}
		handle(frame)
	}
}

func (l *link) send(frame Frame) error {
	raw, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	l.writeMu.Lock()
	defer l.writeMu.Unlock()
	_ = l.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err = l.conn.Write(append(raw, '\n'))
	return err
}

// ask sends a request and waits for its answer within bound.
func (l *link) ask(frame Frame, bound time.Duration) (Frame, error) {
	l.mu.Lock()
	l.next++
	frame.ID = l.next
	answer := make(chan Frame, 1)
	l.waiting[frame.ID] = answer
	l.mu.Unlock()
	forget := func() {
		l.mu.Lock()
		delete(l.waiting, frame.ID)
		l.mu.Unlock()
	}
	if err := l.send(frame); err != nil {
		forget()
		return Frame{}, err
	}
	timer := time.NewTimer(bound)
	defer timer.Stop()
	select {
	case got := <-answer:
		return got, nil
	case <-l.closed:
		forget()
		return Frame{}, errClosed
	case <-timer.C:
		forget()
		return Frame{}, fmt.Errorf("no answer to %s within %s", frame.Op, bound)
	}
}

// answer replies to a request the other side made.
func (l *link) answer(request Frame, reply Frame) error {
	reply.Op, reply.ID = opAnswer, request.ID
	return l.send(reply)
}

func (l *link) close() {
	l.once.Do(func() {
		close(l.closed)
		_ = l.conn.Close()
	})
}

// readLine reads one line within maxFrame.
func readLine(reader *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		chunk, more, err := reader.ReadLine()
		if err != nil {
			return nil, err
		}
		line = append(line, chunk...)
		if len(line) > maxFrame {
			return nil, errors.New("a frame past its bound")
		}
		if !more {
			return line, nil
		}
	}
}
