package cli

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A retry of a turn end answers from the journal of its first attempt, or,
// when that attempt recorded nothing, from what the attempt found owed —
// however late it comes and whichever build prepared it.

// owes says whether api still owes id.
func owes(t *testing.T, dir string, self registry.Session, id string) bool {
	t.Helper()
	waiters, err := mail.ReadWaiters(dir, "api", self.Epoch())
	if err != nil {
		t.Fatal(err)
	}
	for _, waiter := range waiters {
		if slices.Contains(waiter.Messages, id) {
			return true
		}
	}
	return false
}

// A pending end that died after its journal was on record is completed whole
// by the next look: the interim end stays on record for the next unmarked end
// to be asked about, and the mark it used stays too, since no end removes one
// (docs/delivery-turn-end.md#pending-marks).
func TestARecoveredPendingEndKeepsItsInterimOnRecord(t *testing.T) {
	dir, self, web := peerSession(t)
	task := readKind(t, dir, web, mail.Task)
	if err := markPending(dir, "api", self.Epoch(), "the suite is running", 100); err != nil {
		t.Fatal(err)
	}
	event := mail.TurnEnd{ID: "first", Text: "started", Started: 90, Ended: 110, Boundary: boundaryNow(t, dir, self)}
	records := filepath.Join(state.InboxPath(dir, "api"), "pending")
	previous := beforeReports
	beforeReports = func() { _ = os.Chmod(records, 0o500) }
	t.Cleanup(func() { beforeReports = previous; _ = os.Chmod(records, 0o700) })
	failed := completeTurn(dir, self, event, "")
	beforeReports = previous
	if err := os.Chmod(records, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, pending, _ := markWithin(dir, "api", self.Epoch(), 90, 110); failed == nil || !pending {
		t.Fatalf("the end did not die after its journal, or took its mark away: %v, pending %v", failed, pending)
	}
	if err := completeTurn(dir, self, event, ""); err != nil {
		t.Fatal(err)
	}
	for _, report := range reportsTo(t, dir, "web") {
		if slices.Contains(report.InReplyTo, task) && mail.KindOf(report) != mail.Interim {
			t.Fatalf("the pending end was reported as %s", mail.KindOf(report))
		}
	}
	if answersTo(t, dir, task) != 1 || !owes(t, dir, self, task) {
		t.Fatalf("reports %d, still owed %v", answersTo(t, dir, task), owes(t, dir, self, task))
	}
	if line, ok, err := mail.LastInterim(dir, "api", self.Epoch()); err != nil || !ok || line != "the suite is running" {
		t.Fatalf("the interim end is not on record: %q %v %v", line, ok, err)
	}
}
