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
//
// testHomeEnv marks the empty home TestMain gives them, so that a re-executed
// switch leader keeps the home its parent gave the run.
const testHomeEnv = "REWAKE_TEST_HOME"

func TestMain(m *testing.M) {
	// A deferred switch's leader is this binary again (switch_lead.go).
	if os.Getenv(switchLeadEnv) != "" || os.Getenv(switchOutcomeEnv) != "" {
		os.Exit(Run(testCatalog(), nil, os.Stdout, os.Stderr))
	}
	if os.Getenv(testHomeEnv) == "" {
		home, err := os.MkdirTemp("", "rewake-cli-test-home")
		if err != nil {
			_, _ = os.Stderr.WriteString("rewake: no test home: " + err.Error() + "\n")
			os.Exit(1)
		}
		_ = os.Setenv("HOME", home)
		_ = os.Setenv(testHomeEnv, home)
	}
	reportsTurns = func(adapter.Harness) bool { return true }
	if os.Getenv(state.RoomEnv) == "" {
		_ = os.Setenv(state.RoomEnv, state.DefaultRoom)
	}
	// A command that launches reads the person's alias file, modes included,
	// so no test may read the one of whoever runs it: an empty home keeps what
	// the tests see their own. A test that needs a file of its own sets HOME
	// itself.
	code := m.Run()
	if home := os.Getenv(testHomeEnv); home != "" {
		_ = os.RemoveAll(home)
	}
	os.Exit(code)
}
