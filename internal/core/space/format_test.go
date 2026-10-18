package space

import (
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
)

// The record's format on disk is a contract between processes: rewake space
// open writes it, the space host carries it through its steps, and every
// later rewake reads it. A record taken through every step is written as
// exactly this, its keys and its shape; a change of it is a change of the
// format, made on purpose.
func TestTheRecordsFormatOnDisk(t *testing.T) {
	dir := buildtest.Dir(t)
	at := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	checkout := t.TempDir()
	record := Record{
		ID: "0123456789abcdef0123456789abcdef", Name: "api",
		Main:     Run{Name: "main-claude", Run: "4242.1700.boot", PIDNamespace: "pid:[1]"},
		Worker:   Worker{Harness: "claude", Name: "api", Role: "write", SessionID: "0f0e0d0c-0b0a-4908-8706-050403020100"},
		OpenedAt: at,
	}
	if err := Create(dir, record); err != nil {
		t.Fatal(err)
	}
	steps := []func() (Record, error){
		func() (Record, error) { return SetHost(dir, record.ID, "6161.1750.boot") },
		func() (Record, error) { return Opened(dir, record.ID, "5151.1800.boot") },
		func() (Record, error) {
			return SetCheckout(dir, record.ID, Checkout{Path: checkout, Branch: "api"})
		},
		func() (Record, error) { return BeginClosing(dir, record.ID, "main's run ended") },
		func() (Record, error) {
			return Close(dir, record.ID, End{At: at.Add(time.Hour), Why: "main's run ended", Worker: WorkerEnded, Checkout: Seen{Presence: There, Commits: true}})
		},
	}
	for _, step := range steps {
		if _, err := step(); err != nil {
			t.Fatal(err)
		}
	}
	want := `{
  "id": "0123456789abcdef0123456789abcdef",
  "name": "api",
  "state": "kept",
  "main": {
    "name": "main-claude",
    "run": "4242.1700.boot",
    "pidNamespace": "pid:[1]"
  },
  "worker": {
    "harness": "claude",
    "name": "api",
    "role": "write",
    "sessionId": "0f0e0d0c-0b0a-4908-8706-050403020100",
    "run": "5151.1800.boot"
  },
  "checkout": {
    "path": "` + checkout + `",
    "branch": "api"
  },
  "host": "6161.1750.boot",
  "openedAt": "2026-10-18T12:00:00Z",
  "closing": "main's run ended",
  "end": {
    "at": "2026-10-18T13:00:00Z",
    "why": "main's run ended",
    "worker": "ended",
    "checkout": {
      "presence": "there",
      "commits": true
    }
  }
}
`
	if got := onDisk(t, dir, record.ID); got != want {
		t.Fatalf("the record on disk:\n%s\nwant:\n%s", got, want)
	}
}
