// Package registrytest helps tests describe runs of this build.
package registrytest

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// Boot is the id of the boot the test runs in. A session a test publishes
// needs it to be a run of this build: without it the run reads as the earlier
// build's, whose mail this build holds rather than delivers.
func Boot(t testing.TB) string {
	t.Helper()
	boot, err := registry.CurrentBoot()
	if err != nil {
		t.Fatalf("the boot id cannot be read: %v", err)
	}
	return boot
}

// Legacy leaves a session's record in a room of 1.x, or in a root of the
// layout before rooms, as that build wrote it. 2.0 writes neither: no lease
// covers a 1.x room, so the record is written past the state's seam, as the
// fixture of a tree only 2.0's reads may meet.
func Legacy(t testing.TB, dir string, session registry.Session) {
	t.Helper()
	encoded, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	sessions := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessions, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessions, session.Name+".json"), append(encoded, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Remove deletes the record of a name, whoever owns it: a test's way to make a
// run read as ended. Production removes only its own record
// (registry.RemoveOwned).
func Remove(dir, name string) error {
	if !state.ValidName(name) {
		return nil
	}
	return state.WithNameLock(dir, name, func() error {
		err := state.Discard(state.SessionPath(dir, name))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	})
}
