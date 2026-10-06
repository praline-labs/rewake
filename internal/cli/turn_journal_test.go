package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// A turn end writes down what it is about to do before its first report, and
// whoever finds it unfinished completes it before a wait is read again
// (inbox/journal.go). A question's answer is published and cleared the same
// way as a task's report, so each case runs for both.

var journalKinds = []inbox.Kind{inbox.Task, inbox.Question}

// readKind is readFrom for a message of kind.
func readKind(t *testing.T, dir string, web registry.Session, kind inbox.Kind) string {
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
	entries, _ := os.ReadDir(inbox.JournalPath(dir, "api"))
	count := 0
	for _, entry := range entries {
		raw, err := os.ReadFile(filepath.Join(inbox.JournalPath(dir, "api"), entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var journal inbox.TurnJournal
		if err := json.Unmarshal(raw, &journal); err != nil {
			t.Fatal(err)
		}
		if !journal.Done {
			count++
		}
	}
	return count
}

// A turn end that cannot write down its sequence publishes nothing: published
// first, a report whose wait is then not cleared would be answered again once
// a later task joins that wait.
func TestATurnEndThatCannotRecordItsSequencePublishesNothing(t *testing.T) {
	for _, kind := range journalKinds {
		t.Run(string(kind), func(t *testing.T) {
			dir, self, web := toolSession(t)
			earlier := readKind(t, dir, web, kind)
			journals := inbox.JournalPath(dir, "api")
			previous := beforeReports
			beforeReports = func() { _ = os.WriteFile(journals, []byte("in the way"), 0o600) }
			t.Cleanup(func() { beforeReports = previous })
			failed := completeTurn(dir, self, turnResult{Boundary: boundaryNow(t, dir, self), ID: "first", Text: "the first is done"}, "")
			beforeReports = previous
			if failed == nil || answersTo(t, dir, earlier) != 0 {
				t.Fatalf("published without its journal: %v, %d reports", failed, answersTo(t, dir, earlier))
			}
			if err := os.Remove(journals); err != nil {
				t.Fatal(err)
			}
			later := readKind(t, dir, web, kind)
			if err := completeTurn(dir, self, turnResult{Boundary: boundaryNow(t, dir, self), ID: "second", Text: "the second is done"}, ""); err != nil {
				t.Fatal(err)
			}
			if answersTo(t, dir, earlier) != 1 || answersTo(t, dir, later) != 1 {
				t.Fatalf("reports per task: earlier %d, later %d", answersTo(t, dir, earlier), answersTo(t, dir, later))
			}
		})
	}
}

// A turn end that died after writing its sequence and before its report went
// out is completed by the next one: the report is published and the wait
// cleared, and a task read since is reported on its own.
func TestAnUnpublishedReportIsCompletedByTheNextTurnEnd(t *testing.T) {
	for _, kind := range journalKinds {
		t.Run(string(kind), func(t *testing.T) {
			dir, self, web := toolSession(t)
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
			failed := completeTurn(dir, self, turnResult{Boundary: boundaryNow(t, dir, self), ID: "first", Text: "the first is done"}, "")
			beforeReports = previous
			if err := os.Chmod(mailbox, 0o700); err != nil {
				t.Fatal(err)
			}
			if failed == nil || answersTo(t, dir, earlier) != 0 {
				t.Fatalf("the report went out: %v", failed)
			}
			later := readKind(t, dir, web, kind)
			if err := completeTurn(dir, self, turnResult{Boundary: boundaryNow(t, dir, self), ID: "second", Text: "the second is done"}, ""); err != nil {
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

// A run that takes over the waits of an ended one completes that run's
// unfinished turn end first: a wait whose report is out is cleared, not taken
// over and answered again.
func TestAnAdoptedWaitWhoseReportIsOutIsNotAnsweredAgain(t *testing.T) {
	for _, kind := range journalKinds {
		t.Run(string(kind), func(t *testing.T) {
			dir, old, web := toolSession(t)
			id := readKind(t, dir, web, kind)
			// The waits read but take no change: the plan finds nothing
			// unknown, and the clearing fails once the report is out.
			waits := filepath.Join(state.AwaitingPath(dir, "api"), old.Epoch())
			previous := beforeReports
			beforeReports = func() { _ = os.Chmod(waits, 0o500) }
			t.Cleanup(func() { beforeReports = previous; _ = os.Chmod(waits, 0o700) })
			if err := completeTurn(dir, old, turnResult{Boundary: boundaryNow(t, dir, old), ID: "first", Text: "the first is done"}, "thread-a"); err == nil {
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
				adopted, err = inbox.AdoptWaits(dir, "api", resumed.Epoch(), "thread-a")
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if len(adopted) != 0 {
				t.Fatalf("took over a wait whose report is out: %v", adopted)
			}
			if err := completeTurn(dir, resumed, turnResult{Boundary: boundaryNow(t, dir, resumed), ID: "resumed", Text: "the resumed turn is done"}, "thread-a"); err != nil {
				t.Fatal(err)
			}
			if answersTo(t, dir, id) != 1 {
				t.Fatalf("the task got %d reports", answersTo(t, dir, id))
			}
		})
	}
}

// readOnlyJournals makes api's journal directory refuse new records from
// beforeReports on: what is on record still reads, and only the write fails.
func readOnlyJournals(t *testing.T, dir string) {
	t.Helper()
	journals := inbox.JournalPath(dir, "api")
	if err := state.EnsureSubdir(journals); err != nil {
		t.Fatal(err)
	}
	previous := beforeReports
	beforeReports = func() { _ = os.Chmod(journals, 0o500) }
	t.Cleanup(func() { beforeReports = previous; _ = os.Chmod(journals, 0o700) })
}

// writableJournals undoes readOnlyJournals.
func writableJournals(t *testing.T, dir string) {
	t.Helper()
	beforeReports = func() {}
	if err := os.Chmod(inbox.JournalPath(dir, "api"), 0o700); err != nil {
		t.Fatal(err)
	}
}

// An end whose journal could not be written left nothing prepared: its retry
// prepares it again and publishes, rather than find a prepared receipt with
// nothing on record and publish nothing.
func TestAnEndWhoseJournalFailedIsPreparedAgainOnRetry(t *testing.T) {
	for _, kind := range journalKinds {
		t.Run(string(kind), func(t *testing.T) {
			dir, self, web := toolSession(t)
			earlier := readKind(t, dir, web, kind)
			readOnlyJournals(t, dir)
			event := turnResult{Boundary: boundaryNow(t, dir, self), ID: "first", Text: "the first is done"}
			if completeTurn(dir, self, event, "") == nil {
				t.Fatal("the journal was not refused")
			}
			writableJournals(t, dir)
			if err := completeTurn(dir, self, event, ""); err != nil {
				t.Fatal(err)
			}
			if answersTo(t, dir, earlier) != 1 {
				t.Fatalf("the retry published %d reports", answersTo(t, dir, earlier))
			}
		})
	}
}

// An end whose journal is on record answers only from it: a retry after it
// died before any effect, with a later task read since, completes the journal
// and does not prepare the end again over the later task.
func TestAnEndRecordedBeforeItsEffectsIsNotPreparedAgain(t *testing.T) {
	for _, kind := range journalKinds {
		t.Run(string(kind), func(t *testing.T) {
			dir, self, web := toolSession(t)
			earlier := readKind(t, dir, web, kind)
			event := turnResult{Boundary: boundaryNow(t, dir, self), ID: "first", Text: "the first is done"}
			// The journal is written, and its first report cannot be: the
			// state of an end that died between the two.
			blockMailbox(t, dir, "web")
			if completeTurn(dir, self, event, "") == nil {
				t.Fatal("the report was not refused")
			}
			unblockMailbox(t, dir, "web")
			if unfinishedJournals(t, dir) != 1 {
				t.Fatalf("unfinished journals: %d", unfinishedJournals(t, dir))
			}
			later := readKind(t, dir, web, kind)
			if err := completeTurn(dir, self, event, ""); err != nil {
				t.Fatal(err)
			}
			if answersTo(t, dir, earlier) != 1 || answersTo(t, dir, later) != 0 {
				t.Fatalf("the retry answered: earlier %d, later %d", answersTo(t, dir, earlier), answersTo(t, dir, later))
			}
		})
	}
}

// failFirstClear ends api's turn with a kept answer, if any, published and its
// wait not cleared.
func failFirstClear(t *testing.T, kept string) (string, registry.Session, registry.Session, turnResult) {
	t.Helper()
	dir, self, web := toolSession(t)
	readKind(t, dir, web, inbox.Question)
	if kept != "" {
		if err := inbox.KeepAnswer(dir, "api", self.Epoch(), kept); err != nil {
			t.Fatal(err)
		}
	}
	waiter := filepath.Join(state.AwaitingPath(dir, "api"), self.Epoch(), "web")
	previous := beforeReports
	beforeReports = func() { _ = os.Chmod(waiter, 0) }
	t.Cleanup(func() { beforeReports = previous; _ = os.Chmod(waiter, 0o600) })
	event := turnResult{Boundary: boundaryNow(t, dir, self), ID: "first", Text: "the continuation"}
	if completeTurn(dir, self, event, "") == nil {
		t.Fatal("the clearing did not fail")
	}
	beforeReports = previous
	if err := os.Chmod(waiter, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, self, web, event
}

// The kept answer an end published is taken by whoever completes its
// journal: left, it would be joined to the answer of the next question too.
func TestARecoveredEndTakesTheAnswerItPublished(t *testing.T) {
	dir, self, web, _ := failFirstClear(t, "THE_KEPT_ANSWER")
	fresh := readKind(t, dir, web, inbox.Question)
	if err := completeTurn(dir, self, turnResult{Boundary: boundaryNow(t, dir, self), ID: "second", Text: "the new answer"}, ""); err != nil {
		t.Fatal(err)
	}
	for _, report := range reportsTo(t, dir, "web") {
		if slices.Contains(report.InReplyTo, fresh) && strings.Contains(report.Text, "THE_KEPT_ANSWER") {
			t.Fatalf("the next question got the old answer: %q", report.Text)
		}
	}
	if _, kept, err := inbox.KeptAnswer(dir, "api", self.Epoch()); err != nil || kept {
		t.Fatalf("the published answer is still kept: %v %v", kept, err)
	}
}

// A late retry of an end another one completed takes nothing: an answer kept
// since is a later end's.
func TestALateRetryKeepsALaterAnswer(t *testing.T) {
	dir, self, _, event := failFirstClear(t, "")
	if err := completeTurn(dir, self, turnResult{Boundary: boundaryNow(t, dir, self), ID: "second", Text: "nothing more"}, ""); err != nil {
		t.Fatal(err)
	}
	if err := inbox.KeepAnswer(dir, "api", self.Epoch(), "THE_LATER_ANSWER"); err != nil {
		t.Fatal(err)
	}
	if err := completeTurn(dir, self, event, ""); err != nil {
		t.Fatal(err)
	}
	if text, kept, err := inbox.KeptAnswer(dir, "api", self.Epoch()); err != nil || !kept || text != "THE_LATER_ANSWER" {
		t.Fatalf("the late retry took a later answer: %q %v %v", text, kept, err)
	}
}

// The pending mark is taken only once the journal is on record: an end whose
// journal failed is still pending on retry, not reported as finished.
func TestAPendingEndWhoseJournalFailedStaysPending(t *testing.T) {
	dir, self, web := toolSession(t)
	earlier := readKind(t, dir, web, inbox.Task)
	if err := markPending(dir, "api", self.Epoch(), "the suite is running", 100); err != nil {
		t.Fatal(err)
	}
	readOnlyJournals(t, dir)
	event := turnResult{Boundary: boundaryNow(t, dir, self), ID: "first", Text: "started", Started: 90, Ended: 110}
	if completeTurn(dir, self, event, "") == nil {
		t.Fatal("the journal was not refused")
	}
	writableJournals(t, dir)
	if err := completeTurn(dir, self, event, ""); err != nil {
		t.Fatal(err)
	}
	for _, report := range reportsTo(t, dir, "web") {
		if slices.Contains(report.InReplyTo, earlier) && inbox.KindOf(report) != inbox.Interim {
			t.Fatalf("a pending end was reported as %s", inbox.KindOf(report))
		}
	}
	if answersTo(t, dir, earlier) != 1 {
		t.Fatalf("the retry published %d reports", answersTo(t, dir, earlier))
	}
}

// boundaryNow is the read boundary of a completion observed now: the reads
// of self's run so far, and none after. An event keeps it for every retry.
func boundaryNow(t *testing.T, dir string, self registry.Session) *inbox.ReadBoundary {
	t.Helper()
	clock, err := inbox.OpenReadClock(context.Background(), dir, self.Name, self.Epoch())
	if err != nil {
		t.Fatal(err)
	}
	defer clock.Close()
	return clock.Snapshot()
}

// blockMailbox makes the next turn end fail to write into name's mailbox,
// after its journal is on record: the mailbox is closed to writes.
func blockMailbox(t *testing.T, dir, name string) {
	t.Helper()
	mailbox := state.InboxPath(dir, name)
	if err := state.EnsureSubdir(mailbox); err != nil {
		t.Fatal(err)
	}
	previous := beforeReports
	// Closed to writes, not replaced or closed to reading: a look that
	// cannot see whether a letter is there leaves its presence unknown, and
	// the stop that records it is not resolved by the letter being absent
	// once the mailbox comes back (docs/mailbox-records.md#the-stop-on-record).
	beforeReports = func() { _ = os.Chmod(mailbox, 0o500) }
	t.Cleanup(func() { beforeReports = previous; _ = os.Chmod(mailbox, 0o700) })
}

// unblockMailbox puts name's mailbox back.
func unblockMailbox(t *testing.T, dir, name string) {
	t.Helper()
	beforeReports = func() {}
	if err := os.Chmod(state.InboxPath(dir, name), 0o700); err != nil {
		t.Fatal(err)
	}
}
