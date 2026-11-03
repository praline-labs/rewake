package host

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
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
// 2026). Both are the adoption's entry, which Run takes for a request that
// carries a handover; a launch's entry reads the same trap.
func TestAnAdoptionNeitherLaunchesNorReadsTheVersion(t *testing.T) {
	traps, marks := t.TempDir(), t.TempDir()
	trap := fmt.Sprintf("#!/bin/sh\necho \"$*\" > %q\n", filepath.Join(marks, "fake"))
	if err := os.WriteFile(filepath.Join(traps, "fake"), []byte(trap), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", traps+string(os.PathListSeparator)+os.Getenv("PATH"))
	harness := &launchTrap{fakeHarness: &fakeHarness{}}
	request := (&Handover{Name: "api", Epoch: "1.1"}).Request(harness)
	adoption := entryOf(request)
	if version, _, err := adoption.version(request, t.TempDir()); version != "" || err != nil {
		t.Fatalf("the adoption read a version: %q, %v", version, err)
	}
	plan, err := adoption.plan(request, adapter.LaunchRequest{Name: "api"})
	if err != nil || plan.Wake == nil || plan.Command != "" {
		t.Fatalf("the adoption's plan: %+v, %v", plan, err)
	}
	if harness.launched.Load() {
		t.Fatal("the adoption called the adapter's Launch")
	}
	if ran, _ := os.ReadDir(marks); len(ran) > 0 {
		t.Fatal("the adoption read the harness's version")
	}

	_, _, _ = launching{}.version(Request{Harness: harness}, t.TempDir())
	if ran, _ := os.ReadDir(marks); len(ran) == 0 {
		t.Fatal("a launch does not read the trap either: the test shows nothing")
	}
}
