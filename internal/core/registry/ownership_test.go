package registry

import (
	"os"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// Ownership tells a name proven to have passed on from one whose record could
// not be read: a turn end drops its report on the first and only there
// (docs/rules/effects.md, E7).
func TestOwnershipIsReadAsOwnedProvenNotOwnedOrUnknown(t *testing.T) {
	dir := stateDir(t, map[int]uint64{10: 100, 11: 200})
	mine := session("api", 10, 100)
	if err := Publish(dir, mine); err != nil {
		t.Fatal(err)
	}
	if owned, err := Ownership(dir, "api", mine.Epoch()); !owned || err != nil {
		t.Fatalf("the run's own record reads (%v, %v), want owned", owned, err)
	}
	if owned, err := Ownership(dir, "api", session("api", 11, 200).Epoch()); owned || err != nil {
		t.Fatalf("another run's record reads (%v, %v), want proven not owned", owned, err)
	}
	if owned, err := Ownership(dir, "web", mine.Epoch()); owned || err != nil {
		t.Fatalf("no record reads (%v, %v), want proven not owned", owned, err)
	}
	raw, err := os.ReadFile(state.SessionPath(dir, "api"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state.SessionPath(dir, "api"), raw[:len(raw)/2], 0o600); err != nil {
		t.Fatal(err)
	}
	if owned, err := Ownership(dir, "api", mine.Epoch()); owned || err == nil {
		t.Fatalf("a record that cannot be read reads (%v, %v), want unknown, an error", owned, err)
	}
	if OwnsName(dir, "api", mine.Epoch()) {
		t.Fatal("OwnsName answers yes for a record that cannot be read")
	}
}
