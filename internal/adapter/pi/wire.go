package pi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// Both peers number their own requests. Only answer frames consume a waiter;
// a request from the other side may carry the same id.
type frame struct {
	Op           string  `json:"op"`
	ID           int64   `json:"id,omitempty"`
	Protocol     int     `json:"protocol,omitempty"`
	Thread       string  `json:"thread,omitempty"`
	Turn         string  `json:"turn,omitempty"`
	Outcome      string  `json:"outcome,omitempty"`
	Text         string  `json:"text,omitempty"`
	Notice       string  `json:"notice,omitempty"`
	OK           bool    `json:"ok,omitempty"`
	Error        string  `json:"error,omitempty"`
	Steered      bool    `json:"steered,omitempty"`
	State        *sample `json:"state,omitempty"`
	Sequence     uint64  `json:"sequence,omitempty"`
	Pending      bool    `json:"pending,omitempty"`
	Action       string  `json:"action,omitempty"`
	Request      string  `json:"request,omitempty"`
	From         string  `json:"from,omitempty"`
	Focus        string  `json:"focus,omitempty"`
	Reason       string  `json:"reason,omitempty"`
	Detail       string  `json:"detail,omitempty"`
	Open         bool    `json:"open,omitempty"`
	TokensBefore *int64  `json:"tokensBefore,omitempty"`
	TokensAfter  *int64  `json:"tokensAfter,omitempty"`
	// Mirror, in hello's answer, says rewake's face draws the run; Events
	// are the conversation a mirror frame carries (mirror.go).
	Mirror bool          `json:"mirror,omitempty"`
	Events []mirrorEvent `json:"events,omitempty"`
}

const (
	maxFrame        = 4 << 20
	protocolVersion = 2
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
	// Only the reader touches the current turn.
	turn    string
	started int64
}

func newLink(conn net.Conn) *link {
	return &link{conn: conn, waiting: make(map[int64]chan frame), closed: make(chan struct{})}
}

func readFrame(reader *bufio.Reader) (frame, error) {
	var line []byte
	for {
		part, more, err := reader.ReadLine()
		if err != nil {
			return frame{}, err
		}
		if len(line)+len(part) > maxFrame {
			return frame{}, errors.New("extension frame exceeds its bound")
		}
		line = append(line, part...)
		if !more {
			break
		}
	}
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
	for {
		got, err := readFrame(reader)
		if err != nil {
			return
		}
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
	l.mu.Lock()
	l.next++
	request.ID = l.next
	answer := make(chan frame, 1)
	l.waiting[request.ID] = answer
	l.mu.Unlock()
	defer func() { l.mu.Lock(); delete(l.waiting, request.ID); l.mu.Unlock() }()
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
