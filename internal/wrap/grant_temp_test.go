package wrap

import (
	"os"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/grant"
)

// Every directory these tests grant lies in a temporary directory, which the
// hard tier refuses; that refusal has its own test in internal/grant.
func TestMain(m *testing.M) {
	grant.TempRoots = func() []string { return nil }
	os.Exit(m.Run())
}
