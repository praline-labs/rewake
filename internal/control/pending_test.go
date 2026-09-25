package control

import (
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/registry"
)

// Hold hands over the record as it is under the hold, not as it was listed:
// a record closed in between is not held at all.
func TestHoldReadsTheRecordUnderItsHold(t *testing.T) {
	dir := t.TempDir()
	pending := Pending{ID: NewID(), Worker: registry.Session{Name: "worker"}, AskerEpoch: "1.2", AskedAt: time.Now().UTC()}
	release, err := Remember(dir, "lead", pending)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok := Hold(dir, "lead", pending.ID); ok {
		t.Fatal("held while its command holds it")
	}
	release()
	got, let, ok := Hold(dir, "lead", pending.ID)
	if !ok || got.ID != pending.ID || got.Worker.Name != "worker" || got.AskerEpoch != "1.2" || !got.AskedAt.Equal(pending.AskedAt) {
		t.Fatalf("held %v: %+v", ok, got)
	}
	let()
	if err := Forget(dir, "lead", pending.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := Hold(dir, "lead", pending.ID); ok {
		t.Fatal("held a closed record")
	}
}
