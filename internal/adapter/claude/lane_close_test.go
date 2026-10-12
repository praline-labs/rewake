package claude

import (
	"errors"
	"os"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The lane removes its reply socket inside Close, under the lease the
// wrapper holds until then; what its readers do afterwards stays in memory.
// Once the lease is let go, a Close again tries no write.
func TestNothingOfTheLaneWritesAfterClose(t *testing.T) {
	dir := buildtest.Dir(t)
	l := startLane(t, dir)
	l.Close()
	if _, err := os.Lstat(l.reply); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the reply socket outlived Close: %v", err)
	}
	buildtest.Release(dir)
	before := state.NoLeaseRefusals()
	l.Close()
	if refused := state.NoLeaseRefusals() - before; refused != 0 {
		t.Fatalf("the closed lane tried %d writes without the lease", refused)
	}
}
