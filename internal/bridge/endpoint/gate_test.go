package endpoint

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/inbox"
)

// counterGate is a gate over a read clock the test moves by hand.
func counterGate(clock *atomic.Uint64) *Gate {
	return NewGate(func() *inbox.ReadBoundary { return &inbox.ReadBoundary{Through: clock.Load()} })
}

// An acknowledgment that reaches its check after an end was noted, for a call
// made at or before that end, writes nothing; one of a later call may.
func TestAnAcknowledgmentAfterANotedEndWritesNothing(t *testing.T) {
	var clock atomic.Uint64
	gate := counterGate(&clock)
	_, noted := gate.Capture()
	for _, called := range []int64{noted - 1, noted} {
		if leave, ok := gate.Enter(called); ok {
			leave()
			t.Fatalf("a call at %d entered after the end noted at %d", called, noted)
		}
	}
	leave, ok := gate.Enter(noted + 1)
	if !ok {
		t.Fatal("a call after the noted end was refused")
	}
	leave()
	if gate.writing != nil {
		t.Fatal("an acknowledgment that left is still registered")
	}
}

// A capture that finds an acknowledgment writing takes the snapshot that one
// closes with, and nothing a later holder of the mailbox commits: the ack's
// writes are in, a shell read after its release is not. An acknowledgment
// arriving after the note writes nothing.
func TestACaptureTakesTheClosingSnapshotOfTheWritingAcknowledgment(t *testing.T) {
	for _, failed := range []bool{false, true} {
		var clock atomic.Uint64
		clock.Store(5)
		gate := counterGate(&clock)
		leave, ok := gate.Enter(1)
		if !ok {
			t.Fatal("the first acknowledgment was refused")
		}
		captured := make(chan *inbox.ReadBoundary)
		go func() { boundary, _ := gate.Capture(); captured <- boundary }()
		waitFor(t, func() bool { return gate.Noted() != 0 })
		if late, ok := gate.Enter(2); ok {
			late()
			t.Fatal("an acknowledgment checked after the note was let write")
		}
		// The acknowledgment writes; a failure midway closes it all the same.
		clock.Store(7)
		if failed {
			clock.Store(6)
		}
		want := clock.Load()
		leave()
		leave()
		// The next holder of the mailbox, a shell read, commits above it.
		clock.Store(9)
		select {
		case boundary := <-captured:
			if boundary.Through != want {
				t.Fatalf("failed=%v: the boundary is %d, want the acknowledgment's closing %d", failed, boundary.Through, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the capture did not end after the acknowledgment closed")
		}
	}
}

// A capture that waited for an acknowledgment takes that one's snapshot and
// never samples the clock again: every sample here moves the clock, so a
// second one could not go unseen.
func TestACaptureThatWaitedDoesNotSampleAgain(t *testing.T) {
	var clock atomic.Uint64
	gate := NewGate(func() *inbox.ReadBoundary { return &inbox.ReadBoundary{Through: clock.Add(1)} })
	leave, _ := gate.Enter(1)
	captured := make(chan *inbox.ReadBoundary)
	go func() { boundary, _ := gate.Capture(); captured <- boundary }()
	waitFor(t, func() bool { return gate.Noted() != 0 })
	leave()
	if boundary := <-captured; boundary.Through != 1 || clock.Load() != 1 {
		t.Fatalf("the boundary is %d after %d samples", boundary.Through, clock.Load())
	}
}

// With nothing writing, the capture samples the clock while it holds the
// mutex no acknowledgment can register past.
func TestACaptureWithNothingWritingSamplesUnderTheMutex(t *testing.T) {
	var gate *Gate
	held := false
	gate = NewGate(func() *inbox.ReadBoundary {
		held = !gate.mu.TryLock()
		if !held {
			gate.mu.Unlock()
		}
		return &inbox.ReadBoundary{Through: 3}
	})
	boundary, noted := gate.Capture()
	if !held || boundary.Through != 3 || noted == 0 {
		t.Fatalf("sampled under the mutex %v, boundary %+v, noted %d", held, boundary, noted)
	}
}

// Only one acknowledgment is registered at a time: one found writing is one
// that never closed, and nothing is added beside it.
func TestOnlyOneAcknowledgmentWrites(t *testing.T) {
	var clock atomic.Uint64
	gate := counterGate(&clock)
	leave, _ := gate.Enter(1)
	if second, ok := gate.Enter(1); ok {
		second()
		t.Fatal("a second acknowledgment registered beside the first")
	}
	leave()
	if again, ok := gate.Enter(1); !ok {
		t.Fatal("the gate stayed shut after the acknowledgment left")
	} else {
		again()
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("the condition never held")
		}
		time.Sleep(time.Millisecond)
	}
}
