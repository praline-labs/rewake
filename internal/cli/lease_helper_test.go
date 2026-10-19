package cli

import (
	"testing"

	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// launched establishes a room's build the way a launch of this test binary
// does — the real lease, taken for a launch and released — so the change
// forms a test runs there find their own build, as they would after a
// session was launched. It returns the room's directory.
func launched(t *testing.T, base, room string) string {
	t.Helper()
	return buildtest.Room(t, base, room)
}

// currentRoom is the room a command of this process would act in.
func currentRoom(t *testing.T) string {
	t.Helper()
	room, err := state.CurrentRoom()
	if err != nil {
		t.Fatal(err)
	}
	return room
}
