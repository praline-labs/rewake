package cli

import (
	"testing"

	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
)

// launched establishes a room's build the way a launch of this test binary
// does — the real lease, taken for a launch and released — so the change
// forms a test runs there find their own build, as they would after a
// session was launched. It returns the room's directory.
func launched(t *testing.T, base, room string) string {
	t.Helper()
	return buildtest.Room(t, base, room)
}
