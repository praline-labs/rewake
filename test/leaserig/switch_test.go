package leaserig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestConcurrentLaunchesOfOneBuildAllTakeTheRoom holds D4a
// (docs/v2/stage3-state.md#one-build-per-room): launches of one build that
// all read a record not their own — missing, or another build's with nothing
// holding the room — and then ask for the lock exclusive at once all take the
// room. One switches it; the others find their build there and are granted,
// rather than refused by the winner's hold. Each launch is held at the switch
// with the lease shared, so the race happens every round, not by chance. All
// launch as main, and what becomes of them after the lease — one main per
// room — is no business of this test.
func TestConcurrentLaunchesOfOneBuildAllTakeTheRoom(t *testing.T) {
	const launches, rounds = 4, 3
	// The harness outlives the switch's bound, so a launch that took the room
	// keeps holding it shared while the others ask: one that asked only for
	// the lock exclusive, never reading the record again, would be refused.
	harnessCommand := filepath.Join(t.TempDir(), "harness")
	if err := os.WriteFile(harnessCommand, []byte("#!/bin/sh\nsleep 3\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	words := launch("default")
	for i, word := range words {
		if word == "/bin/true" {
			words[i] = harnessCommand
		}
	}
	for _, world := range []string{"no record", "another build"} {
		t.Run(world, func(t *testing.T) {
			for round := range rounds {
				w := newWorld(t)
				if world == "another build" {
					w.heldAt(buildB, "default", "leased", launch("default")...).kill()
				}
				logs := make([]string, launches)
				processes := make([]*process, launches)
				for i := range processes {
					logs[i] = filepath.Join(t.TempDir(), "fault.log")
					processes[i] = w.start(buildA, "default", "other:log="+logs[i]+";other:holdat=lease switch@PAUSE", words...)
				}
				for i, p := range processes {
					if !p.waitHeld() {
						t.Fatalf("round %d: launch %d ended before the switch (%d): %s", round, i, p.code(), p.stderr.String())
					}
				}
				for _, p := range processes {
					if err := os.WriteFile(filepath.Join(p.pause, "go"), nil, 0o600); err != nil {
						t.Fatal(err)
					}
				}
				for i, p := range processes {
					select {
					case <-p.done:
					case <-time.After(30 * time.Second):
						t.Fatalf("round %d: launch %d did not end", round, i)
					}
					log, err := os.ReadFile(logs[i])
					if err != nil {
						t.Fatal(err)
					}
					if !strings.Contains(string(log), "step leased") || strings.Contains(p.stderr.String(), "Rewake: room default") {
						t.Fatalf("round %d: launch %d did not take the room (%d): %s", round, i, p.code(), p.stderr.String())
					}
				}
				if got := w.record("default"); got != buildA.id {
					t.Fatalf("round %d: the room's build is %q", round, got)
				}
			}
		})
	}
}
