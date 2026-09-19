package sessionstate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
	stop := Start(context.Background(), dir, "worker", "epoch", func() Snapshot { calls.Add(1); return Unknown("epoch") })
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
