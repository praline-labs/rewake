package space

import (
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
)

// Worker.Type is optional: a record that has one carries it as "type", and
// TestTheRecordsFormatOnDisk's record, without one, is written exactly as
// before the field.
func TestTheWorkersTypeIsAnOptionalKey(t *testing.T) {
	dir := buildtest.Dir(t)
	record := Record{
		ID: "fedcba9876543210fedcba9876543210", Name: "rev",
		Main:     Run{Name: "main-claude", Run: "4242.1700.boot", PIDNamespace: "pid:[1]"},
		Worker:   Worker{Harness: "claude", Name: "rev", Role: "write", Type: "review"},
		OpenedAt: time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC),
	}
	if err := Create(dir, record); err != nil {
		t.Fatal(err)
	}
	if got := onDisk(t, dir, record.ID); !strings.Contains(got, "\"role\": \"write\",\n    \"type\": \"review\"\n") {
		t.Fatalf("no type key:\n%s", got)
	}
	if loaded, err := Load(dir, record.ID); err != nil || loaded.Worker.Type != "review" {
		t.Fatalf("%v %q", err, loaded.Worker.Type)
	}
}
