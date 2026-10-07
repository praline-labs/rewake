//go:build rewakefixture

package fixture

import (
	"context"
	"errors"
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

// A withdrawal stops an end's attempts: the one under way runs out, and no
// other starts — not even once a new connection makes the turn boundary live
// again, since the end came on the old one.
func TestAWithdrawalStopsAnEndsAttempts(t *testing.T) {
	for _, reconnect := range []bool{false, true} {
		t.Run(map[bool]string{false: "withdrawn", true: "reconnected"}[reconnect], func(t *testing.T) {
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
			finished := make(chan Frame, 1)
			go func() {
				finished <- b.turnEnded(l, Frame{Turn: "t1", End: "t1/e1", Outcome: OutcomeCompleted, Hold: true})
			}()
			<-first
			b.withdraw(l)
			if reconnect {
				b.mu.Lock()
				b.link = newLink(nil)
				b.live = map[string]bool{TurnBoundary: true}
				b.mu.Unlock()
			}
			close(release)
			select {
			case answer := <-finished:
				if calls.Load() != 1 || answer.OK {
					t.Fatalf("the end was attempted again after its withdrawal: %d calls, %+v", calls.Load(), answer)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("the end did not finish after the withdrawal")
			}
		})
	}
}

// A probe the program never answers makes only its capability unavailable:
// the start succeeds within the probe's own bound, with the others live.
func TestASilentProbeCostsOnlyItsCapability(t *testing.T) {
	shorten(t)
	for _, capability := range Served {
		t.Run(capability, func(t *testing.T) {
			b, err := startProgram(t, switchMute+"="+capability)
			if err != nil {
				t.Fatalf("one unanswered probe failed the start: %v", err)
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
