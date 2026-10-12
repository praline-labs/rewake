package session

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func privateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestEpochScopedSnapshotsAndStaleness(t *testing.T) {
	dir := privateDir(t)
	count := uint64(2)
	now := time.Now()
	old := Snapshot{Fresh: true, ContextFresh: true, SettingsFresh: true, PublishedAt: &now, Compactions: &count}
	if err := Save(dir, "worker", "old", old); err != nil {
		t.Fatal(err)
	}
	current := old
	countNew := uint64(9)
	current.Compactions = &countNew
	if err := Save(dir, "worker", "new", current); err != nil {
		t.Fatal(err)
	}
	if got := Load(dir, "worker", "old"); got.Compactions == nil || *got.Compactions != 2 {
		t.Fatal("old message borrowed new epoch")
	}
	if got := Load(dir, "worker", ""); got.Compactions != nil {
		t.Fatal("missing epoch borrowed state")
	}
	expired := now.Add(-3 * time.Second)
	old.PublishedAt = &expired
	if err := Save(dir, "worker", "old", old); err != nil {
		t.Fatal(err)
	}
	if got := Load(dir, "worker", "old"); got.Fresh || got.ContextFresh || got.SettingsFresh {
		t.Fatal("expired heartbeat stayed fresh")
	}
	if got := Load(dir, "worker", "missing"); got.Compactions != nil {
		t.Fatal("unknown became zero")
	}
	tooLong := strings.Repeat("x", maxSnapshotBytes)
	current.Model = &tooLong
	if Save(dir, "worker", "new", current) == nil {
		t.Fatal("unbounded snapshot written")
	}
}

func TestTelemetryStorageFailureDoesNotStopCollection(t *testing.T) {
	dir := privateDir(t)
	if err := os.WriteFile(filepath.Join(dir, "observations"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	stop := Start(context.Background(), dir, "worker", "epoch", func() Snapshot { calls.Add(1); return Unknown("epoch") }, nil)
	defer stop()
	deadline := time.Now().Add(time.Second)
	for calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if calls.Load() < 2 {
		t.Fatal("storage error stopped worker")
	}
	if got := Load(dir, "worker", "epoch"); got.Compactions != nil {
		t.Fatal("failed persistence invented measurements")
	}
}

// The stop publishes what the source holds then, so a run's last change — a
// channel record frozen at its exit — is what stays, whenever the last tick
// came.
func TestTheStopPublishesTheLastWord(t *testing.T) {
	dir := privateDir(t)
	var compactions atomic.Uint64
	stop := Start(context.Background(), dir, "worker", "epoch", func() Snapshot {
		snapshot := Unknown("epoch")
		count := compactions.Load()
		snapshot.Compactions = &count
		return snapshot
	}, nil)
	deadline := time.Now().Add(time.Second)
	for Load(dir, "worker", "epoch").Compactions == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	compactions.Store(7)
	stop()
	if got := Load(dir, "worker", "epoch").Compactions; got == nil || *got != 7 {
		t.Fatalf("the snapshot that stayed counts %v, want the source's 7 at the stop", got)
	}
}

// The stop waits for its worker only so long, but the worker stays counted
// until its last publication ends: a run that keeps the room's lease while
// workers count keeps it through a publication a stalled write delayed past
// the stop.
func TestAStoppedWorkerCountsUntilItsLastWrite(t *testing.T) {
	dir := privateDir(t)
	entered, resume := make(chan struct{}), make(chan struct{})
	var once sync.Once
	var stopping atomic.Bool
	var workers sync.WaitGroup
	stop := Start(context.Background(), dir, "worker", "epoch", func() Snapshot {
		if stopping.Load() {
			once.Do(func() { close(entered); <-resume })
		}
		return Unknown("epoch")
	}, &workers)
	stopping.Store(true)
	stopped := make(chan struct{})
	go func() { stop(); close(stopped) }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the worker never published again")
	}
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		close(resume)
		t.Fatal("the stop waited on a stalled publication")
	}
	done := make(chan struct{})
	go func() { workers.Wait(); close(done) }()
	select {
	case <-done:
		close(resume)
		t.Fatal("the worker stopped counting while it was still publishing")
	case <-time.After(100 * time.Millisecond):
	}
	close(resume)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the worker never stopped counting")
	}
}
