package cli

import (
	"testing"

	"github.com/praline-labs/rewake/internal/infra/build"
)

// launched establishes a room's build the way a launch of this test binary
// does — the real lease, taken for a launch and released — so the change
// forms a test runs there find their own build, as they would after a
// session was launched.
func launched(t *testing.T, base, room string) {
	t.Helper()
	lease, err := build.AcquireLaunch(base, room)
	if err != nil {
		t.Fatalf("establishing the build of room %s: %v", room, err)
	}
	lease.Release()
}
