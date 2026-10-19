// Package buildtest makes the rooms tests write in the way a launch makes
// them. From the root of 2.0 on, no resolver creates a room
// (docs/v2/stage3-state.md#the-roots): only a launch's bootstrap does, under
// the build lease, and no write of a room passes without its lease
// (docs/v2/stage3-state.md#what-is-a-writer). A test that writes a room has it
// made by the same acquisition — the real lease, taken for a launch of the
// running test binary — and holds that lease, shared, until the test ends,
// as a session of the room would: never a switch that lets tests past it.
package buildtest

import (
	"os"
	"sync"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/build"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// Room establishes a room of base as a launch does and returns its
// directory: made, with the running binary's build recorded, so the change
// forms a test runs there find their own build. The test holds the room's
// lease until it ends, so what it writes there to set a scene is written as
// a session's writes are, under the lease; a change form it runs takes its
// own beside it.
func Room(t testing.TB, base, room string) string {
	t.Helper()
	lease, err := build.AcquireLaunch(base, room)
	if err != nil {
		t.Fatalf("establishing room %s: %v", room, err)
	}
	t.Cleanup(lease.Release)
	holds.Lock()
	holds.by[lease.Room()] = append(holds.by[lease.Room()], lease)
	holds.Unlock()
	return lease.Room()
}

// holds are the leases Room took, by room, for Release.
var holds = struct {
	sync.Mutex
	by map[string][]*build.Lease
}{by: map[string][]*build.Lease{}}

// Release lets go, before the test ends, of every hold Room took on the room
// at dir: a test whose scene is set and that now proves what happens while
// another process holds the lease exclusive, or none is held.
func Release(dir string) {
	holds.Lock()
	leases := holds.by[dir]
	delete(holds.by, dir)
	holds.Unlock()
	for _, lease := range leases {
		lease.Release()
	}
}

// Established is Room without the hold: the room is made and its build
// recorded, and the lease is released before it returns. A test that proves
// what a writer does without a lease, or that a launch of another build may
// switch the room, starts here.
func Established(t testing.TB, base, room string) string {
	t.Helper()
	lease, err := build.AcquireLaunch(base, room)
	if err != nil {
		t.Fatalf("establishing room %s: %v", room, err)
	}
	dir := lease.Room()
	lease.Release()
	return dir
}

// Dir is Room for a base of the test's own and the default room: a fresh
// room, held, for a test that only needs somewhere its writes are a room's.
func Dir(t testing.TB) string {
	t.Helper()
	base := t.TempDir()
	// A base is private, as a session's is; a test directory is not.
	if err := os.Chmod(base, 0o700); err != nil {
		t.Fatal(err)
	}
	return Room(t, base, state.DefaultRoom)
}

// Current is Room for the base and the room this process's environment
// names.
func Current(t testing.TB) string {
	t.Helper()
	base, err := state.Base()
	if err != nil {
		t.Fatal(err)
	}
	room, err := state.CurrentRoom()
	if err != nil {
		t.Fatal(err)
	}
	return Room(t, base, room)
}
