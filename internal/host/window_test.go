package host

import (
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/host/delivery"
)

// shortWindow serves the wrappers a test starts with a window a fraction of
// the real one: what these tests ask is whether the wrapper sets a window, and
// the real three seconds would only add to every run.
func shortWindow(t *testing.T) {
	t.Helper()
	previous := delivery.Coalescing
	delivery.Coalescing = delivery.Window{Quiet: time.Second, Cap: 1500 * time.Millisecond}
	t.Cleanup(func() { delivery.Coalescing = previous })
}
