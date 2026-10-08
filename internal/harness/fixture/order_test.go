//go:build rewakefixture

package fixture

import (
	"context"
	"errors"
	"net"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
)

// holdDispatch holds the goroutine a frame of one operation is handled on
// until release is closed, and closes received once that frame is held.
func holdDispatch(t *testing.T, op string) (received, release chan struct{}) {
	t.Helper()
	received, release = make(chan struct{}), make(chan struct{})
	hook := func(frame Frame) {
		if frame.Op == op {
			close(received)
			<-release
		}
	}
	dispatchHook.Store(&hook)
	t.Cleanup(func() { dispatchHook.Store(nil) })
	return received, release
}

func currentLink(b *backend) *link {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.link
}

// An end's boundary is the one at its frame's arrival: a read acknowledged
// while the end waits to be handled is not part of it, or a later task could
// be answered by the earlier end.
func TestAnEndCapturesItsBoundaryWhenItsFrameArrives(t *testing.T) {
	var clock atomic.Uint64
	clock.Store(1)
	seen := make(chan harness.Completion, 1)
	handler := harness.CompletionHandler{
		EndCapture: func() (*inbox.ReadBoundary, int64) { return &inbox.ReadBoundary{Through: clock.Load()}, 100 },
		Publish:    func(_ context.Context, completion harness.Completion) error { seen <- completion; return nil },
	}
	_, p := paired(t, handler, Served...)
	received, release := holdDispatch(t, opTurnEnded)
	p.write(t, Frame{Op: opTurnEnded, ID: 1, Turn: "t1", End: "t1/e1", Outcome: OutcomeCompleted})
	<-received
	clock.Store(2)
	close(release)
	if answer := p.read(t); !answer.OK {
		t.Fatalf("end refused: %+v", answer)
	}
	if completion := <-seen; completion.Boundary == nil || completion.Boundary.Through != 1 {
		t.Fatalf("an end took in a read after its frame arrived: %+v", completion.Boundary)
	}
}

// The same holds for a turn's start: its boundary is the one at its arrival.
func TestATurnStartCapturesItsBoundaryWhenItsFrameArrives(t *testing.T) {
	var clock atomic.Uint64
	clock.Store(1)
	handler := harness.CompletionHandler{Capture: func() *inbox.ReadBoundary { return &inbox.ReadBoundary{Through: clock.Load()} }}
	b, p := paired(t, handler, Served...)
	received, release := holdDispatch(t, opTurnStarted)
	p.write(t, Frame{Op: opTurnStarted, ID: 1, Turn: "t1"})
	<-received
	clock.Store(2)
	close(release)
	if answer := p.read(t); !answer.OK {
		t.Fatalf("start refused: %+v", answer)
	}
	b.mu.Lock()
	record := b.turns["t1"]
	b.mu.Unlock()
	if record.boundary == nil || record.boundary.Through != 1 {
		t.Fatalf("a turn start took in a read after its frame arrived: %+v", record.boundary)
	}
}

// Close returns only once no call into the wrapper's handler runs: the
// wrapper unmaps the read clock those calls capture from after it.
func TestCloseWaitsForACallIntoTheHandler(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	b, _ := paired(t, harness.CompletionHandler{Capture: func() *inbox.ReadBoundary {
		close(entered)
		<-release
		return boundary()
	}}, Served...)
	l := currentLink(b)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		b.turnStarted(l, Frame{Turn: "t1"})
	}()
	<-entered
	closed := make(chan struct{})
	go func() { b.Close(); close(closed) }()
	select {
	case <-closed:
		close(release)
		t.Fatal("Close returned while a capture was still running")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	<-finished
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close did not return after the capture ended")
	}
}

// Nor does it return while a frame is still being handled: what the frame's
// goroutine does next would answer on a backend the wrapper has let go of.
func TestCloseWaitsForAFrameBeingHandled(t *testing.T) {
	b, p := paired(t, harness.CompletionHandler{Capture: boundary}, Served...)
	received, release := holdDispatch(t, opTurnStarted)
	p.write(t, Frame{Op: opTurnStarted, ID: 1, Turn: "t1"})
	<-received
	closed := make(chan struct{})
	go func() { b.Close(); close(closed) }()
	select {
	case <-closed:
		close(release)
		t.Fatal("Close returned while a frame was still being handled")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close did not return after the frame was handled")
	}
}

// Once Close has begun, nothing more is handed to the wrapper.
func TestNothingReachesTheHandlerOnceClosed(t *testing.T) {
	var calls atomic.Int32
	b, _ := paired(t, harness.CompletionHandler{
		Capture: func() *inbox.ReadBoundary { calls.Add(1); return boundary() },
		Publish: func(context.Context, harness.Completion) error { calls.Add(1); return nil },
	}, Served...)
	l := currentLink(b)
	b.Close()
	if answer := b.turnStarted(l, Frame{Turn: "t1"}); answer.OK {
		t.Fatal("a turn start was taken after Close")
	}
	if answer := b.turnEnded(l, Frame{Turn: "t1", End: "t1/e1", Outcome: OutcomeCompleted}); answer.OK || calls.Load() != 0 {
		t.Fatalf("an end was taken after Close: %+v, %d calls", answer, calls.Load())
	}
}

// Close stops calls into the handler from the moment it begins, before the
// connection is withdrawn: a frame taken in between is refused, not handed on.
func TestNothingReachesTheHandlerWhileClosing(t *testing.T) {
	var calls atomic.Int32
	b, _ := paired(t, harness.CompletionHandler{
		Capture: func() *inbox.ReadBoundary { calls.Add(1); return boundary() },
		Publish: func(context.Context, harness.Completion) error { calls.Add(1); return nil },
	}, Served...)
	l := currentLink(b)
	b.mu.Lock()
	b.closing = true
	b.mu.Unlock()
	if answer := b.turnStarted(l, Frame{Turn: "t1"}); answer.OK {
		t.Fatal("a turn start was taken while closing")
	}
	if answer := b.turnEnded(l, Frame{Turn: "t1", End: "t1/e1", Outcome: OutcomeCompleted}); answer.OK || calls.Load() != 0 {
		t.Fatalf("an end was taken while closing: %+v, %d calls", answer, calls.Load())
	}
}

// A withdrawal or a close stops an end's attempts: the one under way runs out,
// and no other starts — not once a new connection makes the turn boundary live
// again, since the end came on the old one, and not in the moment between the
// reader closing the connection and the withdrawal that follows it.
func TestAWithdrawalStopsAnEndsAttempts(t *testing.T) {
	for _, cut := range []string{"withdrawn", "reconnected", "closed"} {
		t.Run(cut, func(t *testing.T) {
			first, release := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			b, _ := paired(t, harness.CompletionHandler{
				Capture: boundary,
				Confirm: func(context.Context, harness.Completion) (string, error) {
					if calls.Add(1) == 1 {
						close(first)
						<-release
						return "", errors.New("the first attempt could not publish")
					}
					return "", nil
				},
			}, Served...)
			l := currentLink(b)
			if cut == "closed" {
				l = unservedLink(t, b)
			}
			finished := make(chan Frame, 1)
			go func() {
				finished <- b.turnEnded(l, Frame{Turn: "t1", End: "t1/e1", Outcome: OutcomeCompleted, Hold: true})
			}()
			<-first
			switch cut {
			case "withdrawn":
				b.withdraw(l)
			case "reconnected":
				b.withdraw(l)
				b.mu.Lock()
				b.link = newLink(nil)
				b.live = map[string]bool{TurnBoundary: true}
				b.mu.Unlock()
			case "closed":
				l.close()
			}
			close(release)
			select {
			case answer := <-finished:
				if calls.Load() != 1 || answer.OK {
					t.Fatalf("the end was attempted again after its connection was %s: %d calls, %+v", cut, calls.Load(), answer)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("the end did not finish after its connection was %s", cut)
			}
		})
	}
}

// A connection already closed takes no turn and no end, though its withdrawal
// has yet to land: its first attempt is refused as a retry would be.
func TestAClosedConnectionTakesNoTurn(t *testing.T) {
	var calls atomic.Int32
	b, _ := paired(t, harness.CompletionHandler{
		Capture: func() *inbox.ReadBoundary { calls.Add(1); return boundary() },
		Confirm: func(context.Context, harness.Completion) (string, error) { calls.Add(1); return "", nil },
	}, Served...)
	l := unservedLink(t, b)
	l.close()
	if answer := b.turnStarted(l, Frame{Turn: "t1"}); answer.OK {
		t.Fatalf("a closed connection started a turn: %+v", answer)
	}
	if answer := b.turnEnded(l, Frame{Turn: "t1", End: "t1/e1", Outcome: OutcomeCompleted, Hold: true}); answer.OK {
		t.Fatalf("a closed connection's end was taken: %+v", answer)
	}
	if calls.Load() != 0 {
		t.Fatalf("a closed connection reached the handler %d times", calls.Load())
	}
}

// unservedLink makes the backend hold a connection no reader serves, with the
// turn boundary live on it. Nothing withdraws it when it closes, which holds
// the moment between the reader closing a connection and the withdrawal that
// follows for as long as a test needs it.
func unservedLink(t *testing.T, b *backend) *link {
	t.Helper()
	near, far := net.Pipe()
	t.Cleanup(func() { _ = far.Close() })
	l := newLink(near)
	b.mu.Lock()
	b.link, b.live = l, map[string]bool{TurnBoundary: true}
	b.mu.Unlock()
	return l
}

// A probe the program never answers makes only its capability unavailable:
// the start succeeds within the probe's own bound, with the others live.
func TestASilentProbeCostsOnlyItsCapability(t *testing.T) {
	shorten(t)
	for _, capability := range Served {
		t.Run(capability, func(t *testing.T) {
			began := time.Now()
			b, err := startProgram(t, switchMute+"="+capability)
			if err != nil {
				t.Fatalf("one unanswered probe failed the start: %v", err)
			}
			// The hello and one probe's bound, with room for a loaded machine;
			// a probe allowed more than its own bound would take far longer.
			if took := time.Since(began); took > 3*readinessBound {
				t.Fatalf("the start took %s with one probe silent, past its bound of %s", took, readinessBound)
			}
			want := slices.DeleteFunc(slices.Clone(Served), func(c string) bool { return c == capability })
			if got := b.Live(); !slices.Equal(got, want) {
				t.Fatalf("live %v, want %v", got, want)
			}
		})
	}
}

// A sample that arrives before Telemetry is proven is not the session's state
// once its probe succeeds: when it was taken cannot be told.
func TestActivityBeforeTelemetryIsProvenStaysUnknown(t *testing.T) {
	b, _ := paired(t, harness.CompletionHandler{}, Wake)
	b.activity(currentLink(b), Frame{State: "working"})
	b.mu.Lock()
	b.live[Telemetry] = true
	b.mu.Unlock()
	if state := b.SessionState(); state.Activity != nil {
		t.Fatalf("a sample taken before telemetry was live became the state: %+v", state)
	}
}

// Nor is a sample from a connection the backend no longer holds, even when
// Telemetry is live on the one that replaced it.
func TestActivityFromAWithdrawnConnectionIsDropped(t *testing.T) {
	b, _ := paired(t, harness.CompletionHandler{}, Served...)
	old := currentLink(b)
	b.mu.Lock()
	b.link = newLink(nil)
	b.mu.Unlock()
	b.activity(old, Frame{State: "working"})
	if state := b.SessionState(); state.Activity != nil {
		t.Fatalf("a sample from the old connection became the state: %+v", state)
	}
}
