//go:build rewakefixture

package fixture

import (
	"errors"
	"os"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// What the backend writes of the room it writes before Close returns, under
// the lease the wrapper holds until then: the socket is gone when Close
// returns, and once the lease is let go a Close again tries no write. A write
// that still came later would meet ErrNoLease, not land.
func TestNothingOfTheBackendWritesAfterClose(t *testing.T) {
	b, err := startProgram(t)
	if err != nil {
		t.Fatal(err)
	}
	b.Close()
	if _, err := os.Lstat(b.socket); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the socket outlived Close: %v", err)
	}
	buildtest.Release(b.dir)
	before := state.NoLeaseRefusals()
	b.Close()
	if refused := state.NoLeaseRefusals() - before; refused != 0 {
		t.Fatalf("the closed backend tried %d writes without the lease", refused)
	}
}
