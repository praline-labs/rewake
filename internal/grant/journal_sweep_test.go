package grant

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/registry/registrytest"
	"github.com/praline-labs/rewake/internal/state"
)

// Saving a journal removes those of runs that have ended, once they are old
// enough that no run could be between its registration and its record; the
// journal of a live run stays, however old.
func TestSavingSweepsTheJournalsOfEndedRuns(t *testing.T) {
	dir := t.TempDir()
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	live := registry.Session{Name: "writer", ServicePID: os.Getpid(), ServiceStart: start, Boot: registrytest.Boot(t), CWD: dir, StartedAt: time.Now()}
	if err := state.EnsureSubdir(state.SessionsPath(dir)); err != nil {
		t.Fatal(err)
	}
	if err := registry.Publish(dir, live); err != nil {
		t.Fatal(err)
	}
	entries := []Entry{{Path: "/w/a", Message: "m1", Outcome: Granted}}
	for _, run := range [][2]string{{"writer", live.Epoch()}, {"gone", "1.1"}, {"fresh", "2.2"}} {
		if err := Save(dir, run[0], run[1], entries); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * sweepGrace)
	for _, run := range [][2]string{{"writer", live.Epoch()}, {"gone", "1.1"}} {
		if err := os.Chtimes(journalPath(dir, run[0], run[1]), old, old); err != nil {
			t.Fatal(err)
		}
	}
	// Saved again, as the next delivery saves it.
	if err := Save(dir, "writer", live.Epoch(), entries); err != nil {
		t.Fatal(err)
	}
	if Load(dir, "writer", live.Epoch()) == nil {
		t.Error("the journal of a live run was swept")
	}
	if Load(dir, "fresh", "2.2") == nil {
		t.Error("a young journal was swept")
	}
	if _, err := os.Stat(journalPath(dir, "gone", "1.1")); !os.IsNotExist(err) {
		t.Errorf("the journal of an ended run stayed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "grants")); err != nil {
		t.Fatal(err)
	}
}
