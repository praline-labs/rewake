package host

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/registry"
)

// launchTrap is a harness an adoption may only serve: its Launch and the
// read of its version are traps.
type launchTrap struct {
	*fakeHarness
	launched atomic.Bool
}

func (l *launchTrap) Launch(request adapter.LaunchRequest) (adapter.LaunchPlan, error) {
	l.launched.Store(true)
	return l.fakeHarness.Launch(request)
}

func (l *launchTrap) Version() adapter.VersionGate {
	return adapter.VersionGate{Minimum: "1.0.0", Args: []string{"--version"}}
}

// A run handed over is planned with its adapter's Serve alone: its Launch,
// which prepares a harness not yet started, is never called, and the
// harness's version is never read — the program on PATH may have changed
// since the start, and a refusal would end a running session (November 3,
// 2026). The run is adopted and served.
func TestAnAdoptionNeitherLaunchesNorReadsTheVersion(t *testing.T) {
	traps, marks := t.TempDir(), t.TempDir()
	trap := fmt.Sprintf("#!/bin/sh\necho \"$*\" > %q\n", filepath.Join(marks, "fake"))
	if err := os.WriteFile(filepath.Join(traps, "fake"), []byte(trap), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", traps+string(os.PathListSeparator)+os.Getenv("PATH"))
	harness := &launchTrap{fakeHarness: &fakeHarness{}}
	dir, worker, done := holdRun(t, harness)

	deadline := time.Now().Add(5 * time.Second)
	for {
		got, err := registry.Load(dir, worker.Name)
		if err != nil {
			t.Fatal(err)
		}
		if got.HarnessPID != 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the run handed over was never adopted")
		}
		time.Sleep(20 * time.Millisecond)
	}
	assertHolding(t, done)
	if harness.launched.Load() {
		t.Fatal("the adoption called the adapter's Launch")
	}
	if ran, _ := os.ReadDir(marks); len(ran) > 0 {
		t.Fatal("the adoption read the harness's version")
	}
}
