package space

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
)

// A worker's type is gone from new records, which write none; a record of
// before still reads, its obsolete field ignored, so no state migration is
// needed for existing spaces.
func TestANewWorkerRecordOmitsTheOldTypeAndAnOldOneReads(t *testing.T) {
	dir := buildtest.Dir(t)
	record := Record{
		ID: "fedcba9876543210fedcba9876543210", Name: "rev",
		Main:     Run{Name: "main-claude", Run: "4242.1700.boot", PIDNamespace: "pid:[1]"},
		Worker:   Worker{Harness: "claude", Name: "rev", Role: "write"},
		OpenedAt: time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC),
	}
	if err := Create(dir, record); err != nil {
		t.Fatal(err)
	}
	if got := onDisk(t, dir, record.ID); strings.Contains(got, "\"type\"") {
		t.Fatalf("a new record carries the old type:\n%s", got)
	}
	if loaded, err := Load(dir, record.ID); err != nil || loaded.Name != "rev" {
		t.Fatalf("%v %+v", err, loaded)
	}

	// The record of before this change, as it was written, with a type.
	old := strings.Replace(onDisk(t, dir, record.ID), `"role": "write"`, `"role": "write",
    "type": "review"`, 1)
	path := filepath.Join(dir, "spaces", record.ID+".json")
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir, record.ID)
	if err != nil || loaded.Worker.Name != "rev" || loaded.Worker.Role != "write" {
		t.Fatalf("an old record no longer reads: %v %+v", err, loaded)
	}
	var raw map[string]any
	if json.Unmarshal([]byte(old), &raw) != nil {
		t.Fatal("the old record is not JSON")
	}
}
