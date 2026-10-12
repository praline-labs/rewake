// Package buildtest makes the rooms tests write in the way a launch makes
// them. From the root of 2.0 on, no resolver creates a room
// (docs/v2/stage3-state.md#the-roots): only a launch's bootstrap does, under
// the build lease. A test that writes a room has it made by the same
// acquisition — the real lease, taken for a launch of the running test binary
// and released — never by a switch that lets tests past the lease.
package buildtest

import (
	"testing"

	"github.com/praline-labs/rewake/internal/infra/build"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// Room establishes a room of base as a launch does and returns its
// directory: made, with the running binary's build recorded, so the change
// forms a test runs there find their own build.
func Room(t testing.TB, base, room string) string {
	t.Helper()
	lease, err := build.AcquireLaunch(base, room)
	if err != nil {
		t.Fatalf("establishing room %s: %v", room, err)
	}
	dir := lease.Room()
	lease.Release()
	return dir
}

// Current is Room for the base and the room this process's environment
// names.
func Current(t testing.TB) string {
	t.Helper()
	base, err := state.Base()
	if err != nil {
		t.Fatal(err)
	}
	return Room(t, base, state.CurrentRoom())
}
