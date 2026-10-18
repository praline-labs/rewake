package cli

import (
	"os"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
)

// The sessions these tests make stand on a harness of no particular contract,
// recorded under a real adapter's id, and they owe reports: every harness
// reports its turns here, but in the tests of the refusals
// (turn_reporting_test.go).
func TestMain(m *testing.M) {
	reportsTurns = func(adapter.Harness) bool { return true }
	os.Exit(m.Run())
}
