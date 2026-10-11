package mail

import (
	"os"
	"sync/atomic"
	"testing"

	"github.com/praline-labs/rewake/internal/core/mail/mailtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// failStatusWrites makes writing the status of these messages fail, while
// what is on disk stays readable, until the returned function is called. The
// write fails where it asks state.Fault, before any of it is written.
func failStatusWrites(t *testing.T, dir, name string, ids ...string) func() {
	t.Helper()
	var blocked atomic.Bool
	blocked.Store(true)
	paths := map[string]bool{}
	for _, id := range ids {
		paths[mailtest.StatusPath(dir, name, id)] = true
	}
	previous := state.Fault
	state.Fault = func(op, path string) error {
		if op == state.OpWrite && blocked.Load() && paths[path] {
			return os.ErrPermission
		}
		if previous != nil {
			return previous(op, path)
		}
		return nil
	}
	t.Cleanup(func() { state.Fault = previous })
	return func() { blocked.Store(false) }
}
