package cli

import (
	"os"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The sessions these tests make stand on a harness of no particular contract,
// recorded under a real adapter's id, and they owe reports: every harness
// reports its turns here, but in the tests of the refusals
// (turn_reporting_test.go).
//
// They act in the room "default" unless they choose one: a command outside a
// session takes the room of its working directory, which here would be the
// package's, and a test that means that room sets REWAKE_ROOM empty itself.
func TestMain(m *testing.M) {
	reportsTurns = func(adapter.Harness) bool { return true }
	if os.Getenv(state.RoomEnv) == "" {
		_ = os.Setenv(state.RoomEnv, state.DefaultRoom)
	}
	os.Exit(m.Run())
}
