package cli

import (
	"os"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/grant"
)

// Every directory these tests grant lies in a temporary directory, which the
// hard tier refuses; that refusal has its own test in internal/grant.
//
// The sessions these tests make stand on a harness of no particular contract,
// recorded under a real adapter's id, and they owe reports: every harness
// reports its turns here, but in the tests of the refusals
// (turn_reporting_test.go).
func TestMain(m *testing.M) {
	grant.TempRoots = func() []string { return nil }
	reportsTurns = func(adapter.Harness) bool { return true }
	os.Exit(m.Run())
}
