package host

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The run takes the room's build lease before it claims a name and lets it
// go only after its cleanup: when the lease is released the session's record
// and socket are gone, so a launch of another build never switches a room
// while a run of the old one still writes there.
func TestHostLeaseOutlivesCleanup(t *testing.T) {
	base := t.TempDir()
	if err := os.Chmod(base, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(state.DirEnv, base)
	// Made and let go: the run's own lease is the only one.
	dir := buildtest.Established(t, base, state.DefaultRoom)
	var claimed registry.Session
	leasedAtClaim := false
	releases := 0
	var leftAtRelease []string
	before := state.Fault
	t.Cleanup(func() { state.Fault = before })
	state.Fault = func(op, path string) error {
		if op != state.OpNote || path != "lease release" || claimed.Name == "" {
			return nil
		}
		releases++
		if _, err := registry.Load(dir, claimed.Name); !errors.Is(err, registry.ErrNotFound) {
			leftAtRelease = append(leftAtRelease, "the session record")
		}
		if _, err := os.Lstat(claimed.Socket); claimed.OwnsSocket && err == nil {
			leftAtRelease = append(leftAtRelease, "the socket")
		}
		if !state.Leased(dir) {
			leftAtRelease = append(leftAtRelease, "(the lease was already revoked)")
		}
		return nil
	}
	_, err := Run(context.Background(), Request{
		Harness: &fakeHarness{script: "exit 0"}, Dir: dir, Name: "api",
		OnClaimed: func(session registry.Session) error {
			claimed = session
			leasedAtClaim = state.Leased(dir)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !leasedAtClaim {
		t.Fatal("the name was claimed without the room's lease")
	}
	if releases != 1 || len(leftAtRelease) > 0 {
		t.Fatalf("%d releases; still there when the lease went: %v", releases, leftAtRelease)
	}
	if state.Leased(dir) {
		t.Fatal("the lease outlived the run")
	}
}
