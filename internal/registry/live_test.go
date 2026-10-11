package registry

import (
	"errors"
	"slices"
	"testing"
)

// Two writers changing different fields of one record keep each other's
// change, and a run that no longer owns the name changes nothing.
func TestModifyChangesTheRecordAsItStands(t *testing.T) {
	dir := stateDir(t, map[int]uint64{10: 100})
	record := session("api", 10, 100)
	if err := Publish(dir, record); err != nil {
		t.Fatal(err)
	}
	epoch := record.Epoch()
	if err := Modify(dir, "api", epoch, func(s *Session) { s.Live = []string{"wake"} }); err != nil {
		t.Fatal(err)
	}
	if err := Modify(dir, "api", epoch, func(s *Session) { s.HarnessPID = 42 }); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir, "api")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.HarnessPID != 42 || !slices.Equal(loaded.Live, []string{"wake"}) || !loaded.HasLive("wake") || loaded.HasLive("control") {
		t.Fatalf("the record: %+v", loaded)
	}
	if err := Modify(dir, "api", "1.1", func(s *Session) { s.Live = nil }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another run's change: %v", err)
	}
	if loaded, _ := Load(dir, "api"); !loaded.HasLive("wake") {
		t.Fatal("another run's change was written")
	}
}
