package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A turn end writes its reports before it clears the waits they answer, and a
// wait left standing is answered by the next end or not taken over when its
// report is out (mail/turn_reports.go, mail/answered.go). A question's answer
// is published and cleared the same way as a task's report, so each case runs
// for both.

var journalKinds = []mail.Kind{mail.Task, mail.Question}

// readKind is readFrom for a message of kind.
func readKind(t *testing.T, dir string, web registry.Session, kind mail.Kind) string {
	t.Helper()
	current, _ := registry.Lookup(dir, "api")
	id := rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": current.Epoch(), "kind": string(kind), "text": "rerun"})
	if code, _, errOut := run("inbox"); code != ExitOK {
		t.Fatalf("inbox: %d %s", code, errOut)
	}
	return id
}

// answersTo counts the reports to web that answer id.
func answersTo(t *testing.T, dir, id string) int {
	t.Helper()
	count := 0
	for _, report := range reportsTo(t, dir, "web") {
		if slices.Contains(report.InReplyTo, id) {
			count++
		}
	}
	return count
}

// unfinishedJournals counts the journals of api not marked done.
func unfinishedJournals(t *testing.T, dir string) int {
	t.Helper()
	entries, _ := os.ReadDir(mail.JournalPath(dir, "api"))
	count := 0
	for _, entry := range entries {
		raw, err := os.ReadFile(filepath.Join(mail.JournalPath(dir, "api"), entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var journal mail.TurnJournal
		if err := json.Unmarshal(raw, &journal); err != nil {
			t.Fatal(err)
		}
		if !journal.Done {
			count++
		}
	}
	return count
}

// A turn end whose report could not be written leaves its wait standing, and
// the next end answers it: once, with the task read since.
func TestAnUnpublishedReportIsCompletedByTheNextTurnEnd(t *testing.T) {
	for _, kind := range journalKinds {
		t.Run(string(kind), func(t *testing.T) {
			dir, self, web := peerSession(t)
			earlier := readKind(t, dir, web, kind)
			mailbox := state.InboxPath(dir, "web")
			if err := state.EnsureSubdir(mailbox); err != nil {
				t.Fatal(err)
			}
			previous := beforeReports
			// Closed to writes: closed to reading, the look at the letter
			// would leave its presence unknown, which no later absence
			// resolves.
			beforeReports = func() { _ = os.Chmod(mailbox, 0o500) }
			t.Cleanup(func() { beforeReports = previous; _ = os.Chmod(mailbox, 0o700) })
			failed := completeTurn(dir, self, mail.TurnEnd{Boundary: boundaryNow(t, dir, self), ID: "first", Text: "the first is done"}, "")
			beforeReports = previous
			if err := os.Chmod(mailbox, 0o700); err != nil {
				t.Fatal(err)
			}
			if failed == nil || answersTo(t, dir, earlier) != 0 {
				t.Fatalf("the report went out: %v", failed)
			}
			later := readKind(t, dir, web, kind)
			if err := completeTurn(dir, self, mail.TurnEnd{Boundary: boundaryNow(t, dir, self), ID: "second", Text: "the second is done"}, ""); err != nil {
				t.Fatal(err)
			}
			if answersTo(t, dir, earlier) != 1 || answersTo(t, dir, later) != 1 {
				t.Fatalf("reports per task: earlier %d, later %d", answersTo(t, dir, earlier), answersTo(t, dir, later))
			}
			if left := unfinishedJournals(t, dir); left != 0 {
				t.Fatalf("a completed sequence left %d journals unfinished", left)
			}
		})
	}
}

// A run that takes over the waits of an ended one does not take over a wait
// whose report that run wrote before it could clear the wait: the task is
// answered once.
func TestAnAdoptedWaitWhoseReportIsOutIsNotAnsweredAgain(t *testing.T) {
	for _, kind := range journalKinds {
		t.Run(string(kind), func(t *testing.T) {
			dir, old, web := peerSession(t)
			id := readKind(t, dir, web, kind)
			// The waits read but take no change: the plan finds nothing
			// unknown, and the clearing fails once the report is out.
			waits := filepath.Join(state.AwaitingPath(dir, "api"), old.Epoch())
			previous := beforeReports
			beforeReports = func() { _ = os.Chmod(waits, 0o500) }
			t.Cleanup(func() { beforeReports = previous; _ = os.Chmod(waits, 0o700) })
			if err := completeTurn(dir, old, mail.TurnEnd{Boundary: boundaryNow(t, dir, old), ID: "first", Text: "the first is done"}, "thread-a"); err == nil {
				t.Fatal("a turn end whose clearing failed answered success")
			}
			beforeReports = previous
			if err := os.Chmod(waits, 0o700); err != nil {
				t.Fatal(err)
			}
			if answersTo(t, dir, id) != 1 {
				t.Fatalf("the report did not go out once: %d", answersTo(t, dir, id))
			}
			threads := filepath.Join(state.InboxPath(dir, "api"), "threads")
			if err := os.MkdirAll(threads, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(threads, id), []byte("thread-a"), 0o600); err != nil {
				t.Fatal(err)
			}
			resumed := otherRun(t, dir, "api")
			var adopted []string
			if err := state.WithMailboxLock(context.Background(), dir, "api", func() error {
				var err error
				adopted, err = mail.AdoptWaits(dir, "api", resumed.Epoch(), "thread-a")
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if len(adopted) != 0 {
				t.Fatalf("took over a wait whose report is out: %v", adopted)
			}
			if err := completeTurn(dir, resumed, mail.TurnEnd{Boundary: boundaryNow(t, dir, resumed), ID: "resumed", Text: "the resumed turn is done"}, "thread-a"); err != nil {
				t.Fatal(err)
			}
			if answersTo(t, dir, id) != 1 {
				t.Fatalf("the task got %d reports", answersTo(t, dir, id))
			}
		})
	}
}

// boundaryNow is the read boundary of a completion observed now: the reads
// of self's run so far, and none after. An event keeps it for every retry.
func boundaryNow(t *testing.T, dir string, self registry.Session) *mail.ReadBoundary {
	t.Helper()
	clock, err := mail.OpenReadClock(context.Background(), dir, self.Name, self.Epoch())
	if err != nil {
		t.Fatal(err)
	}
	defer clock.Close()
	return clock.Snapshot()
}
