package spacehost

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/space"
	"github.com/praline-labs/rewake/internal/host/terminal"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A space host that runs below main's wrapper starts no worker: it is what
// main's end would take down with it, and it is the very process the
// authority's check from below would let act for main. It closes the record,
// the worker never started, saying why — here main's run is this process
// itself, so the space host is main's.
func TestASpaceHostBelowMainStartsNoWorker(t *testing.T) {
	dir := buildtest.Dir(t)
	self, err := selfRun()
	if err != nil {
		t.Fatal(err)
	}
	id, err := space.NewID()
	if err != nil {
		t.Fatal(err)
	}
	if err := space.Create(dir, space.Record{
		ID: id, Name: "api",
		Main:     space.Run{Name: "main-fixture", Run: self},
		Worker:   space.Worker{Harness: "fixture", Name: "api", Role: "write"},
		OpenedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "started")
	h := &host{spec: Spec{
		Base: filepath.Dir(dir), Room: filepath.Base(dir), Dir: dir, ID: id,
		Args: []string{"-c", "touch " + marker}, Cwd: t.TempDir(), Size: terminal.Size{Cols: 80, Rows: 24},
	}}
	if err := h.start(); err == nil {
		t.Fatal("the space host below main went on")
	}
	record, err := space.Load(dir, id)
	if err != nil {
		t.Fatal(err)
	}
	if record.State != space.Closed || record.Host != self || record.End.Worker != space.WorkerNeverStarted || !strings.Contains(record.End.Why, "outside main's run") {
		t.Fatalf("the record: %+v %+v", record, record.End)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the worker started")
	}
	if _, err := os.Lstat(state.SpaceSocketPath(dir, id)); err == nil {
		t.Fatal("the space host serves a terminal")
	}
}
