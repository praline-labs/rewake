package cli

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// A retry of a turn end answers from the journal of its first attempt, or,
// when that attempt recorded nothing, from what the attempt found owed —
// however late it comes and whichever build prepared it.

// owes says whether api still owes id.
func owes(t *testing.T, dir string, self registry.Session, id string) bool {
	t.Helper()
	waiters, err := inbox.ReadWaiters(dir, "api", self.Epoch())
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

// aDayPasses ages every record of both mailboxes past what the sweep keeps.
func aDayPasses(t *testing.T, dir string) {
	t.Helper()
	old := time.Now().Add(-25 * time.Hour)
	for _, name := range []string{"api", "web"} {
		err := filepath.WalkDir(state.InboxPath(dir, name), func(path string, entry fs.DirEntry, err error) error {
			if errors.Is(err, fs.ErrNotExist) && path == state.InboxPath(dir, name) {
				return nil
			}
			if err != nil || !entry.Type().IsRegular() {
				return err
			}
			return os.Chtimes(path, old, old)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// sweepAsServer runs the sweep a session's server runs when it starts.
func sweepAsServer(t *testing.T, dir string, session registry.Session) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan struct{})
	server := inbox.Server{Dir: dir, Name: session.Name, Epoch: session.Epoch(), Ready: cancel}
	go func() { server.Serve(ctx); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("the server did not stop")
	}
}

// A pending end that died after its journal was on record is completed whole
// by the next look: the interim end stays on record for the next unmarked end
// to be asked about, and the mark it used stays too, since no end removes one
// (docs/turn-end-recovery.md#pending-marks).
func TestARecoveredPendingEndKeepsItsInterimOnRecord(t *testing.T) {
	dir, self, web := toolSession(t)
	task := readKind(t, dir, web, inbox.Task)
	if err := markPending(dir, "api", self.Epoch(), "the suite is running", 100); err != nil {
		t.Fatal(err)
	}
	event := inbox.TurnEnd{ID: "first", Text: "started", Started: 90, Ended: 110, Boundary: boundaryNow(t, dir, self)}
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
		if slices.Contains(report.InReplyTo, task) && inbox.KindOf(report) != inbox.Interim {
			t.Fatalf("the pending end was reported as %s", inbox.KindOf(report))
		}
	}
	if answersTo(t, dir, task) != 1 || !owes(t, dir, self, task) {
		t.Fatalf("reports %d, still owed %v", answersTo(t, dir, task), owes(t, dir, self, task))
	}
	if line, ok, err := inbox.LastInterim(dir, "api", self.Epoch()); err != nil || !ok || line != "the suite is running" {
		t.Fatalf("the interim end is not on record: %q %v %v", line, ok, err)
	}
}

// A retry of a completed end a day later, after the sweep, finds its end
// completed: the live run's receipts and journals are kept while it lives.
func TestALateRetryAfterTheSweepPublishesNothing(t *testing.T) {
	for _, kind := range journalKinds {
		t.Run(string(kind), func(t *testing.T) {
			dir, self, web := toolSession(t)
			first := readKind(t, dir, web, kind)
			event := inbox.TurnEnd{Boundary: boundaryNow(t, dir, self), ID: "first", Text: "FIRST_END_TEXT"}
			if err := completeTurn(dir, self, event, ""); err != nil {
				t.Fatal(err)
			}
			fresh := readKind(t, dir, web, kind)
			aDayPasses(t, dir)
			sweepAsServer(t, dir, self)
			if err := completeTurn(dir, self, event, ""); err != nil {
				t.Fatal(err)
			}
			if a, b := answersTo(t, dir, first), answersTo(t, dir, fresh); a != 1 || b != 0 {
				t.Fatalf("the late retry published: first %d, fresh %d", a, b)
			}
			if !owes(t, dir, self, fresh) {
				t.Fatal("the late retry settled a task it never saw")
			}
		})
	}
}

// A retry of an end whose journal failed answers only what its first attempt
// found owed: a question read after another end reported the original task
// is not answered with the first end's text.
func TestARetryAnswersOnlyWhatItsFirstAttemptSaw(t *testing.T) {
	dir, self, web := toolSession(t)
	original := readKind(t, dir, web, inbox.Task)
	readOnlyJournals(t, dir)
	event := inbox.TurnEnd{Boundary: boundaryNow(t, dir, self), ID: "first", Text: "FIRST_END_TEXT"}
	if completeTurn(dir, self, event, "") == nil {
		t.Fatal("the journal was not refused")
	}
	writableJournals(t, dir)
	if err := completeTurn(dir, self, inbox.TurnEnd{Boundary: boundaryNow(t, dir, self), ID: "second", Text: "the second is done"}, ""); err != nil {
		t.Fatal(err)
	}
	fresh := readKind(t, dir, web, inbox.Question)
	if err := completeTurn(dir, self, event, ""); err != nil {
		t.Fatal(err)
	}
	for _, report := range reportsTo(t, dir, "web") {
		if slices.Contains(report.InReplyTo, fresh) && strings.Contains(report.Text, "FIRST_END_TEXT") {
			t.Fatalf("the fresh question got the first end's text: %q", report.Text)
		}
	}
	if answersTo(t, dir, original) != 1 || answersTo(t, dir, fresh) != 0 || !owes(t, dir, self, fresh) {
		t.Fatalf("reports: original %d, fresh %d; fresh owed %v", answersTo(t, dir, original), answersTo(t, dir, fresh), owes(t, dir, self, fresh))
	}
}
