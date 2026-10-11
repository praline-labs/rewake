package mail

import (
	"context"
	"fmt"
	"testing"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// sweepAsLiveRun runs the sweep of finished mail the way the delivery server
// runs it for a live run: inside the mailbox lock, with the run read there as
// the name's live run. That read is the test's precondition, not a second
// gate: a run that is not live fails the test, since the server would sweep
// nothing then, and the gate itself is the server's to test. t.Error rather
// than t.Fatal, so a sweep may run in a goroutine of its own.
func sweepAsLiveRun(t *testing.T, dir, name, epoch string) {
	t.Helper()
	if err := state.WithMailboxLock(context.Background(), dir, name, func() error {
		current, err := registry.LookupReadOnly(dir, name)
		if err != nil || epoch == "" || current.Epoch() != epoch {
			return fmt.Errorf("the sweeping run %q is not the live run of %s (%v): the server would sweep nothing", epoch, name, err)
		}
		SweepFinishedLocked(dir, name, epoch)
		return nil
	}); err != nil {
		t.Error(err)
	}
}
