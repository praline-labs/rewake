package pi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/praline-labs/rewake/internal/core/sessiontools"
)

// Both peers number their own requests. Only answer frames consume a waiter;
// a request from the other side may carry the same id.
type frame struct {
	Op       string  `json:"op"`
	ID       int64   `json:"id,omitempty"`
	Protocol int     `json:"protocol,omitempty"`
	Thread   string  `json:"thread,omitempty"`
	Turn     string  `json:"turn,omitempty"`
	Outcome  string  `json:"outcome,omitempty"`
	Text     string  `json:"text,omitempty"`
	Notice   string  `json:"notice,omitempty"`
	OK       bool    `json:"ok,omitempty"`
	Error    string  `json:"error,omitempty"`
	Steered  bool    `json:"steered,omitempty"`
	State    *sample `json:"state,omitempty"`
	Sequence uint64  `json:"sequence,omitempty"`
	Pending  bool    `json:"pending,omitempty"`
	Action   string  `json:"action,omitempty"`
	Request  string  `json:"request,omitempty"`
	From     string  `json:"from,omitempty"`
	Focus    string  `json:"focus,omitempty"`
	// Code names the extension's check that refused a control command, and
	// Native the raw result of its native call; the host words both
	// (control.go). Levels are the thinking levels the face's shift+tab
	// cycles (native.go).
	Code   string          `json:"code,omitempty"`
	Native json.RawMessage `json:"native,omitempty"`
	Levels []string        `json:"levels,omitempty"`
	// Mirror, in hello's answer, says rewake's face draws the run; Events
	// are pi's own events an events frame carries (translate.go).
	Mirror bool  `json:"mirror,omitempty"`
	Events []raw `json:"events,omitempty"`
	// The tools (tools.go): the table, in hello's answer and a tools frame;
	// a call's plan, asked by tool-plan with the call's arguments and
	// answered by its argv and bound or a refusal; its raw end, which
	// tool-result carries and whose answer is the text and isError.
	Tools     []toolEntry       `json:"tools,omitempty"`
	Tool      string            `json:"tool,omitempty"`
	Arguments json.RawMessage   `json:"arguments,omitempty"`
	Argv      []string          `json:"argv,omitempty"`
	TimeoutMs int64             `json:"timeoutMs,omitempty"`
	Refused   string            `json:"refused,omitempty"`
	Ran       *sessiontools.Ran `json:"ran,omitempty"`
	IsError   bool              `json:"isError,omitempty"`
}

const (
	maxFrame        = 4 << 20
	protocolVersion = 3
)

var errClosed = errors.New("the extension connection closed")

type link struct {
	conn    net.Conn
	writeMu sync.Mutex
	mu      sync.Mutex
	next    int64
	waiting map[int64]chan frame
	closed  chan struct{}
	once    sync.Once
	// Only the reader writes the current turn, under mu, which Busy reads
	// it under.
	turn    string
	started int64
	// asking counts the host's requests waiting for an answer that are not
	// the poll of the state: an answer only this image would take (Busy).
	asking int
	// halt, while a handover holds the reader, takes what it read of a
	// frame; frozen refuses the host's requests then (handover.go).
	halt   chan []byte
	resume chan struct{}
	frozen bool
}

func newLink(conn net.Conn) *link {
	return &link{conn: conn, waiting: make(map[int64]chan frame), closed: make(chan struct{})}
}

func readFrame(reader *bufio.Reader) (frame, error) {
	got, _, err := readFrom(reader, nil)
	return got, err
}

// readFrom reads the frame whose first bytes line holds. On a read's error it
// answers what it took of the frame, which a halt for a handover keeps
// (handover.go): the reader has nothing else.
func readFrom(reader *bufio.Reader, line []byte) (frame, []byte, error) {
	for {
		part, err := reader.ReadSlice('\n')
		line = append(line, part...)
		if len(bytes.TrimRight(line, "\r\n")) > maxFrame {
			return frame{}, nil, errors.New("extension frame exceeds its bound")
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if err != nil {
			return frame{}, line, err
		}
		break
	}
	got, err := parseFrame(bytes.TrimRight(line, "\r\n"))
	return got, nil, err
}

func parseFrame(line []byte) (frame, error) {
	var got frame
	if err := json.Unmarshal(line, &got); err != nil {
		return frame{}, errors.New("invalid extension frame")
	}
	if got.Op == "" || got.ID <= 0 {
		return frame{}, errors.New("extension frame needs an operation and positive id")
	}
	return got, nil
}

func (l *link) serve(reader *bufio.Reader, handle func(frame)) {
	defer l.close()
	var partial []byte
	for {
		got, rest, err := readFrom(reader, partial)
		if err != nil {
			var halted bool
			if partial, halted = l.halted(err, rest); halted {
				continue
			}
			return
		}
		partial = nil
		if got.Op != "answer" {
			handle(got)
			continue
		}
		l.mu.Lock()
		waiter := l.waiting[got.ID]
		delete(l.waiting, got.ID)
		l.mu.Unlock()
		if waiter != nil {
			waiter <- got
		}
	}
}

func (l *link) send(got frame) error {
	raw, err := json.Marshal(got)
	if err != nil {
		return err
	}
	if len(raw) > maxFrame {
		return errors.New("extension frame exceeds its bound")
	}
	l.writeMu.Lock()
	defer l.writeMu.Unlock()
	if err := l.conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	_, err = l.conn.Write(append(raw, '\n'))
	return err
}

func (l *link) ask(ctx context.Context, request frame) (frame, error) {
	// A poll lost to an exec is asked again by the next image.
	counted := request.Op != "state"
	l.mu.Lock()
	if l.frozen {
		l.mu.Unlock()
		return frame{}, errHanding
	}
	l.next++
	request.ID = l.next
	answer := make(chan frame, 1)
	l.waiting[request.ID] = answer
	if counted {
		l.asking++
	}
	l.mu.Unlock()
	defer func() {
		l.mu.Lock()
		delete(l.waiting, request.ID)
		if counted {
			l.asking--
		}
		l.mu.Unlock()
	}()
	if err := l.send(request); err != nil {
		return frame{}, err
	}
	select {
	case got := <-answer:
		return got, nil
	case <-l.closed:
		return frame{}, errClosed
	case <-ctx.Done():
		return frame{}, fmt.Errorf("no answer to %s: %w", request.Op, ctx.Err())
	}
}

func (l *link) answer(request, reply frame) {
	reply.Op, reply.ID = "answer", request.ID
	if err := l.send(reply); err != nil {
		l.close()
	}
}

func (l *link) close() { l.once.Do(func() { close(l.closed); _ = l.conn.Close() }) }
func (l *link) gone() bool {
	select {
	case <-l.closed:
		return true
	default:
		return false
	}
}
