package host

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A worker of the run that is inside a write when the harness exits — the
// availability publisher here, paused in a write to main's mailbox — keeps
// the room's lease held until it stops, however long that takes: Run may
// return first, but no write of the run comes after the lease is released.
func TestRunKeepsTheLeaseUntilItsWorkersStop(t *testing.T) {
	dir := stateDir(t)
	_ = availabilityPeer(t, dir, "worker-fixture", role.General.ID, 1, true)
	signal := filepath.Join(t.TempDir(), "exit-harness")
	entered, resume, releasedNote := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var paused, releasedOnce sync.Once
	var released, resumed atomic.Bool
	var mu sync.Mutex
	var wrong []string
	before := state.Fault
	state.Fault = func(op, path string) error {
		if op == state.OpNote && path == "lease release" {
			if !resumed.Load() {
				mu.Lock()
				wrong = append(wrong, "the lease was released while the publisher was paused")
				mu.Unlock()
			}
			released.Store(true)
			releasedOnce.Do(func() { close(releasedNote) })
		}
		if op == state.OpWrite && strings.Contains(path, "/inbox/main-fake/") && strings.HasSuffix(path, ".json") {
			paused.Do(func() { close(entered); <-resume })
		}
		if released.Load() && (op == state.OpWrite || op == state.OpPublish || op == state.OpRemove) {
			mu.Lock()
			wrong = append(wrong, "after the release: "+op+" "+path)
			mu.Unlock()
		}
		return nil
	}
	finished := make(chan error, 1)
	go func() {
		_, err := Run(context.Background(), Request{
			Harness: &fakeHarness{script: "while [ ! -e '" + signal + "' ]; do sleep 0.02; done"},
			Dir:     dir, Name: "main", Role: role.Main,
		})
		finished <- err
	}()
	// The hook is restored only once the run let go of everything: the
	// publisher resumed, Run returned and the lease was released.
	defer func() {
		resumed.Store(true)
		select {
		case <-resume:
		default:
			close(resume)
		}
		select {
		case <-releasedNote:
		case <-time.After(5 * time.Second):
			t.Error("the lease was never released")
		}
		state.Fault = before
	}()
	select {
	case <-entered:
	case err := <-finished:
		t.Fatalf("the run ended before the publication: %v", err)
	case <-time.After(5 * time.Second):
		_ = os.WriteFile(signal, nil, 0o600)
		t.Fatal("the availability publication did not start")
	}
	if err := os.WriteFile(signal, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	// Longer than every bounded stop of the run: the lease stays all the same.
	time.Sleep(1500 * time.Millisecond)
	if released.Load() {
		t.Fatal("the lease was released while the publisher was paused")
	}
	resumed.Store(true)
	close(resume)
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not finish after the publisher resumed")
	}
	select {
	case <-releasedNote:
	case <-time.After(5 * time.Second):
		t.Fatal("the lease was never released")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(wrong) > 0 {
		t.Fatal(strings.Join(wrong, "; "))
	}
}

// A live set's retry that is writing when the run stops counts among the
// run's workers until its write ends, and a stopped live set writes nothing
// more: neither a retry waking after the stop nor a report made after it.
func TestALiveSetWritesNothingAfterItsStop(t *testing.T) {
	dir, session := liveRun(t)
	var workers sync.WaitGroup
	live := newLiveSet(dir, session.Name, session.Epoch())
	live.workers = &workers
	both := []adapter.Capability{adapter.WakeCapability, adapter.TurnBoundaryCapability}
	live.report(both)
	record := state.SessionPath(dir, session.Name)
	entered, resume := make(chan struct{}), make(chan struct{})
	var paused sync.Once
	var reported, stopped atomic.Bool
	var late atomic.Int32
	previous := state.Fault
	t.Cleanup(func() { state.Fault = previous })
	state.Fault = func(op, path string) error {
		if op != state.OpWrite || path != record {
			return nil
		}
		if stopped.Load() {
			late.Add(1)
		}
		if reported.Load() {
			// The retry: it pauses inside its write, and fails it.
			paused.Do(func() { close(entered); <-resume })
		}
		return errors.New("a transient write failure")
	}
	live.report(nil)
	reported.Store(true)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the retry never wrote")
	}
	live.stop()
	done := make(chan struct{})
	go func() { workers.Wait(); close(done) }()
	select {
	case <-done:
		close(resume)
		t.Fatal("the retry stopped counting while it was writing")
	case <-time.After(100 * time.Millisecond):
	}
	stopped.Store(true)
	close(resume)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the retry never stopped counting")
	}
	live.report(both)
	time.Sleep(3 * liveRetry)
	if n := late.Load(); n != 0 {
		t.Fatalf("%d writes after the live set stopped", n)
	}
	if got := recordedLive(t, dir).Live; !slices.Equal(got, []string{"wake", "turn-boundary"}) {
		t.Fatalf("the record after the stop: %v", got)
	}
}
