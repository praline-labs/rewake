//go:build rewakefixture

package fixture

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
)

// paired is a backend whose program is the test itself, on the other end of
// a pipe, with the given capabilities live — the state a finished exchange
// leaves. What is tested here is what the adapter does with a live
// connection; the exchange that makes one is exchange_test.go's.
func paired(t *testing.T, handler harness.CompletionHandler, live ...string) (*backend, *peer) {
	t.Helper()
	near, far := net.Pipe()
	b := newBackend("", nil, nil, "", "", "e1")
	b.handler, b.note = handler, func(string) {}
	b.ctx, b.cancel = context.WithCancel(context.Background())
	l := newLink(near)
	b.link, b.thread = l, programThrd
	for _, capability := range live {
		b.live[capability] = true
	}
	go l.serve(bufio.NewReader(near), func(frame Frame) { b.handle(l, frame) })
	p := &peer{conn: far, reader: bufio.NewReader(far)}
	t.Cleanup(func() { l.close(); _ = far.Close(); b.cancel() })
	return b, p
}

// peer is the program's side of a paired backend.
type peer struct {
	conn   net.Conn
	reader *bufio.Reader
	next   int64
}

func (p *peer) write(t *testing.T, frame Frame) {
	t.Helper()
	raw, _ := json.Marshal(frame)
	if _, err := p.conn.Write(append(raw, '\n')); err != nil {
		t.Fatal(err)
	}
}

func (p *peer) read(t *testing.T) Frame {
	t.Helper()
	_ = p.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	line, err := readLine(p.reader)
	if err != nil {
		t.Fatal(err)
	}
	var frame Frame
	if err := json.Unmarshal(line, &frame); err != nil {
		t.Fatal(err)
	}
	return frame
}

// ask sends a request and reads its answer; nothing else may come between.
func (p *peer) ask(t *testing.T, frame Frame) Frame {
	t.Helper()
	p.next++
	frame.ID = p.next
	p.write(t, frame)
	answer := p.read(t)
	if answer.Op != opAnswer || answer.ID != frame.ID {
		t.Fatalf("asked %s, got %+v", frame.Op, answer)
	}
	return answer
}

// calls records what the core was handed: each completion, and the context
// it came with as the call began.
type calls struct {
	mu        sync.Mutex
	confirmed []harness.Completion
	published []harness.Completion
	deadlines []time.Time
	expired   []bool
}

func (c *calls) enter(ctx context.Context) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	deadline, _ := ctx.Deadline()
	c.deadlines = append(c.deadlines, deadline)
	c.expired = append(c.expired, ctx.Err() != nil)
	return len(c.deadlines)
}

func boundary() *inbox.ReadBoundary { return &inbox.ReadBoundary{} }

func TestAnEndIsConfirmedWhenItCanBeHeld(t *testing.T) {
	var c calls
	handler := harness.CompletionHandler{
		Capture: boundary,
		Confirm: func(ctx context.Context, completion harness.Completion) (string, error) {
			c.enter(ctx)
			c.mu.Lock()
			c.confirmed = append(c.confirmed, completion)
			c.mu.Unlock()
			return "the reason", nil
		},
		Publish: func(ctx context.Context, completion harness.Completion) error {
			c.enter(ctx)
			c.mu.Lock()
			c.published = append(c.published, completion)
			c.mu.Unlock()
			return nil
		},
	}
	_, p := paired(t, handler, Served...)
	if answer := p.ask(t, Frame{Op: opTurnStarted, Turn: "t1"}); !answer.OK {
		t.Fatalf("turn start: %+v", answer)
	}
	held := p.ask(t, Frame{Op: opTurnEnded, Turn: "t1", End: "t1/1", Outcome: OutcomeCompleted, Text: "done", Hold: true})
	if !held.OK || held.Reason != "the reason" || len(c.confirmed) != 1 || len(c.published) != 0 {
		t.Fatalf("a holdable end: %+v, confirmed %d, published %d", held, len(c.confirmed), len(c.published))
	}
	// RW_SHIM_NO_HOLD: an end the program cannot continue is published.
	plain := p.ask(t, Frame{Op: opTurnEnded, Turn: "t1", End: "t1/2", Outcome: OutcomeCompleted, Text: "done"})
	if !plain.OK || plain.Reason != "" || len(c.confirmed) != 1 || len(c.published) != 1 {
		t.Fatalf("an end without hold: %+v, confirmed %d, published %d", plain, len(c.confirmed), len(c.published))
	}
	first := c.confirmed[0]
	if first.ID != ID+"/t1/1" || first.Thread != programThrd || first.Kind != inbox.Finished || first.Text != "done" ||
		first.Boundary == nil || first.Started == 0 || first.Ended < first.Started {
		t.Fatalf("the completion: %+v", first)
	}
	if c.published[0].ID == first.ID {
		t.Fatal("two ends of one turn with one id")
	}
}

// The same end sent again — its answer lost — is the same completion: the
// same id, boundary and times, so the core knows it for the one it answered.
func TestAnEndSentAgainIsTheSameCompletion(t *testing.T) {
	var seen []harness.Completion
	var mu sync.Mutex
	handler := harness.CompletionHandler{
		Capture: boundary,
		Confirm: func(_ context.Context, completion harness.Completion) (string, error) {
			mu.Lock()
			defer mu.Unlock()
			seen = append(seen, completion)
			return "", nil
		},
	}
	_, p := paired(t, handler, Served...)
	p.ask(t, Frame{Op: opTurnStarted, Turn: "t1"})
	end := Frame{Op: opTurnEnded, Turn: "t1", End: "t1/1", Outcome: OutcomeInterrupted, Hold: true}
	p.ask(t, end)
	time.Sleep(5 * time.Millisecond)
	p.ask(t, end)
	if len(seen) != 2 || seen[0] != seen[1] || seen[0].Kind != inbox.Stopped {
		t.Fatalf("the end sent again: %+v", seen)
	}
}

func TestAnEndWithAnUnknownOutcomeIsRefused(t *testing.T) {
	_, p := paired(t, harness.CompletionHandler{Publish: func(context.Context, harness.Completion) error { return nil }}, Served...)
	if answer := p.ask(t, Frame{Op: opTurnEnded, Turn: "t1", End: "t1/1", Outcome: "elsewhere"}); answer.OK {
		t.Fatal("an end of no known outcome was taken")
	}
	if answer := p.ask(t, Frame{Op: opTurnEnded, Turn: "t1", Outcome: OutcomeFailed}); answer.OK {
		t.Fatal("an end that names no event was taken")
	}
}

// E3: an attempt that fails is retried as a new call with its own deadline,
// never by stretching the one that failed, and a bounded number of times.
func TestATurnEndIsRetriedWithItsOwnDeadline(t *testing.T) {
	var c calls
	handler := harness.CompletionHandler{
		Capture: boundary,
		Confirm: func(ctx context.Context, _ harness.Completion) (string, error) {
			if c.enter(ctx) == 1 {
				<-ctx.Done()
				return "", ctx.Err()
			}
			return "", nil
		},
	}
	b, p := paired(t, handler, Served...)
	b.endWait = 150 * time.Millisecond
	answer := p.ask(t, Frame{Op: opTurnEnded, Turn: "t1", End: "t1/1", Outcome: OutcomeCompleted, Hold: true})
	if !answer.OK || len(c.deadlines) != 2 {
		t.Fatalf("the retried end: %+v after %d calls", answer, len(c.deadlines))
	}
	if c.expired[1] || !c.deadlines[1].After(c.deadlines[0].Add(endBackoff)) {
		t.Fatalf("the retry ran under the first attempt's deadline: %v then %v (expired %v)", c.deadlines[0], c.deadlines[1], c.expired[1])
	}
}

func TestATurnEndThatNeverTakesIsRefusedAfterItsAttempts(t *testing.T) {
	var c calls
	failed := errors.New("the journal is unavailable")
	handler := harness.CompletionHandler{
		Capture: boundary,
		Publish: func(ctx context.Context, _ harness.Completion) error { c.enter(ctx); return failed },
	}
	_, p := paired(t, handler, Served...)
	answer := p.ask(t, Frame{Op: opTurnEnded, Turn: "t1", End: "t1/1", Outcome: OutcomeFailed})
	if answer.OK || len(c.deadlines) != endAttempts {
		t.Fatalf("an end that never took: %+v after %d calls", answer, len(c.deadlines))
	}
}

func TestActivityIsTheSessionState(t *testing.T) {
	backend, other := paired(t, harness.CompletionHandler{}, Served...)
	other.write(t, Frame{Op: opActivity, State: "working"})
	eventually(t, "the activity", func() bool {
		state := backend.SessionState()
		return state.Activity != nil && *state.Activity == "working" && state.ActivityFresh
	})
}
