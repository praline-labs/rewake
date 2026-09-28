package wrap

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
	"github.com/praline-labs/rewake/internal/state"
)

func TestAvailabilityScanDoesNotCleanDeadPeerUnderNameLock(t *testing.T) {
	dir := stateDir(t)
	main := availabilityPeer(t, dir, "main-fixture", role.Main.ID, 1, true)
	peer := availabilityPeer(t, dir, "worker-fixture", role.Write.ID, 2, true)
	peer.PIDNamespace = proc.Namespace()
	peer.HarnessPID = 2147483647
	peer.HarnessStart = 1
	if err := registry.Update(dir, peer); err != nil {
		t.Fatal(err)
	}
	held, release, unlocked := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(unlocked)
		_ = state.WithNameLock(dir, peer.Name, func() error { close(held); <-release; return nil })
	}()
	<-held
	done := make(chan error, 1)
	go func() {
		if err := announceAvailable(context.Background(), dir, main, map[string]registry.Session{}); err != nil {
			done <- err
			return
		}
		err := putAvailability(context.Background(), dir, main, peer, availabilityMessage(main, peer))
		if errors.Is(err, registry.ErrNotFound) {
			err = nil
		}
		done <- err
	}()
	completed := false
	select {
	case err := <-done:
		if err != nil {
			t.Error(err)
		}
		completed = true
	case <-time.After(300 * time.Millisecond):
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	mailboxErr := state.WithMailboxLock(ctx, dir, main.Name, func() error { return nil })
	cancel()
	close(release)
	<-unlocked
	if !completed {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("scan remained blocked")
		}
	}
	if !completed || mailboxErr != nil {
		t.Fatal("availability identity observation acquired cleanup locks")
	}
	if _, err := registry.Load(dir, peer.Name); err != nil {
		t.Fatal("optional scan deleted registry evidence")
	}
}
