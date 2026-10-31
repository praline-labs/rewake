package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/session"
	"github.com/praline-labs/rewake/internal/core/space"
)

// The pane's top row is where the session works now: the directory its
// harness reports, with the branch read there once it left its space's
// checkout, else where it started with the checkout's branch. The strip's
// limits are the five-hour and seven-day windows the harness reported.
func TestARowNamesWhereTheSessionWorksNowAndTheAccountsLimits(t *testing.T) {
	root := t.TempDir()
	checkout := filepath.Join(root, "wt")
	moved := filepath.Join(root, "other")
	for dir, head := range map[string]string{checkout: "ref: refs/heads/fix\n", moved: "ref: refs/heads/elsewhere\n"} {
		if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte(head), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	records := []space.Record{{State: space.Open, Worker: space.Worker{Name: "api", Harness: "claude"}, Checkout: space.Checkout{Path: checkout, Branch: "fix"}}}
	at := time.UnixMilli(1_790_000_000_000)
	view := func(telemetry *session.Snapshot) sessionView {
		return sessionView{Telemetry: telemetry, Session: registry.Session{Name: "api-claude", CWD: checkout}}
	}

	started := sessionRowsOf(root, records, []sessionView{view(nil)}, "", nil)[0]
	if started.Path != checkout || started.Branch == nil || *started.Branch != "fix" || started.Limits != nil {
		t.Fatalf("where it started: %+v", started)
	}

	limits := []session.RateLimit{{Kind: "five_hour", Percent: 37, ResetsAt: &at}, {Kind: "seven_day", Percent: 9}, {Kind: "seven_day_opus", Percent: 50}}
	row := sessionRowsOf(root, records, []sessionView{view(&session.Snapshot{Cwd: &moved, RateLimits: limits})}, "", func(string, string) string { return "" })[0]
	if row.Cwd != checkout || row.Path != moved || row.Branch == nil || *row.Branch != "elsewhere" {
		t.Fatalf("after a move: %+v", row)
	}
	want := []LimitRow{{Window: "5h", Percent: 37, ResetsAt: ptr(int(at.UnixMilli()))}, {Window: "7d", Percent: 9}}
	if len(row.Limits) != len(want) {
		t.Fatalf("the limits: %+v", row.Limits)
	}
	for i, got := range row.Limits {
		if got.Window != want[i].Window || got.Percent != want[i].Percent || (got.ResetsAt == nil) != (want[i].ResetsAt == nil) || (got.ResetsAt != nil && *got.ResetsAt != *want[i].ResetsAt) {
			t.Errorf("limit %d: %+v, want %+v", i, got, want[i])
		}
	}
}
