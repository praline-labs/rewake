package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// A receipt a build before the turn journal wrote belongs to a run of that
// build that has ended; a run of this build meets it at adoption or at its
// next turn end and converts it, trusting it with no more than it proves
// (docs/turn-end-recovery-findings.md#what-the-earlier-probes-now-expect).

// earlierAPIRun is api's run of the earlier build: an epoch without a boot.
const earlierAPIRun = "4194000.7"

// legacy(rewake <2026-09-30): a receipt prepared by a build before the turn journal holds its reports and waits itself; remove when no session started by an earlier build is registered
type earlierBuildReceipt struct {
	ID       string
	Done     bool
	Prepared bool
	Waiters  []inbox.Waiter
	Reports  []inbox.Message
}

// earlierTask is a task from web that api's earlier run read and owes.
func earlierTask(t *testing.T, dir string, web registry.Session) string {
	t.Helper()
	task := inbox.Message{ID: inbox.NewID(), From: "web", FromEpoch: web.Epoch(), To: "api", ToEpoch: earlierAPIRun, Kind: inbox.Task, Text: "earlier work", CreatedAt: time.Now()}
	if err := inbox.MarkRead(dir, "api", earlierAPIRun, task, true); err != nil {
		t.Fatal(err)
	}
	return task.ID
}

// writeEarlierBuildReceipt leaves what that build left for a turn end of its
// run that answered task: done, it published the report and died before it
// cleared the wait. It answers the report.
func writeEarlierBuildReceipt(t *testing.T, dir string, web registry.Session, task string, done bool) inbox.Message {
	t.Helper()
	waiters, err := inbox.ReadWaiters(dir, "api", earlierAPIRun)
	if err != nil {
		t.Fatal(err)
	}
	report := inbox.Message{ID: inbox.NewID(), From: "api", FromEpoch: earlierAPIRun, To: "web", ToEpoch: web.Epoch(), Kind: inbox.Finished, Text: "EARLIER_BUILD_TEXT", InReplyTo: []string{task}, CreatedAt: time.Now()}
	if done {
		if err := inbox.PutOnce(dir, report); err != nil {
			t.Fatal(err)
		}
	}
	writeReceiptFile(t, dir, earlierBuildReceipt{ID: inbox.NewID(), Prepared: true, Done: done, Waiters: waiters, Reports: []inbox.Message{report}})
	return report
}

func writeReceiptFile(t *testing.T, dir string, receipt earlierBuildReceipt) string {
	t.Helper()
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.EnsureSubdir(inbox.TurnsPath(dir, "api")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(inbox.TurnsPath(dir, "api"), inbox.NewID())
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// earlierOwes says whether api's earlier run still owes id.
func earlierOwes(t *testing.T, dir, id string) bool {
	t.Helper()
	return owes(t, dir, registry.Session{Name: "api", ServicePID: 4194000, ServiceStart: 7}, id)
}

// A done receipt, or one whose report is found, is completed by the next turn
// end without a second copy; one whose report is neither found nor proven
// absent stops the mailbox: nothing is published, no wait cleared, and the
// turn end, the inbox and pending are refused until main settles it.
func TestAnEarlierBuildsReceiptIsRecoveredByTheNextEnd(t *testing.T) {
	for _, done := range []bool{false, true} {
		t.Run(fmt.Sprintf("done=%v", done), func(t *testing.T) {
			dir, self, web := toolSession(t)
			otherRun(t, dir, "lead")
			markMain(t, dir, "lead")
			earlier := earlierTask(t, dir, web)
			report := writeEarlierBuildReceipt(t, dir, web, earlier, done)
			fresh := readKind(t, dir, web, inbox.Question)
			end := turnResult{Boundary: boundaryNow(t, dir, self), ID: "second", Text: "the second is done"}
			err := completeTurn(dir, self, end, "")
			if done {
				if err != nil {
					t.Fatal(err)
				}
				if answersTo(t, dir, earlier) != 1 || answersTo(t, dir, fresh) != 1 || earlierOwes(t, dir, earlier) || owes(t, dir, self, fresh) {
					t.Fatalf("reports: earlier %d, fresh %d", answersTo(t, dir, earlier), answersTo(t, dir, fresh))
				}
				return
			}
			var stop *inbox.StoppedError
			if !errors.As(err, &stop) {
				t.Fatalf("the turn end went on past an unknown report: %v", err)
			}
			if answersTo(t, dir, earlier) != 0 || answersTo(t, dir, fresh) != 0 || !earlierOwes(t, dir, earlier) || !owes(t, dir, self, fresh) {
				t.Fatal("a stopped mailbox changed")
			}
			turnStarted(t, dir, self, markAt-1)
			for _, words := range [][]string{{"inbox"}, {"pending", "waits for the settle"}} {
				if code, _, errOut := run(words...); code != ExitFailed || !strings.Contains(errOut, "rewake settle api "+report.ID) {
					t.Fatalf("%s on a stopped mailbox: %d %s", words[0], code, errOut)
				}
			}
			if code, _, errOut := run("inbox", "--peek"); code != ExitOK {
				t.Fatalf("peek on a stopped mailbox: %d %s", code, errOut)
			}
			if notes := reportsTo(t, dir, "lead"); len(notes) != 1 {
				t.Fatalf("main got %d notes, want one", len(notes))
			}
			if code, out, errOut := asPerson(t, "settle", "api", report.ID, "--undelivered"); code != ExitOK {
				t.Fatalf("settle: %d %s %s", code, out, errOut)
			}
			if code, _, errOut := asPerson(t, "settle", "api", report.ID, "--undelivered"); code != ExitOK {
				t.Fatalf("the same words again: %d %s", code, errOut)
			}
			if code, _, _ := asPerson(t, "settle", "api", report.ID, "--delivered"); code != ExitUsage {
				t.Fatalf("the opposite words: %d", code)
			}
			if err := completeTurn(dir, self, end, ""); err != nil {
				t.Fatal(err)
			}
			if answersTo(t, dir, earlier) != 1 || answersTo(t, dir, fresh) != 1 || earlierOwes(t, dir, earlier) || owes(t, dir, self, fresh) {
				t.Fatalf("reports after the settle: earlier %d, fresh %d", answersTo(t, dir, earlier), answersTo(t, dir, fresh))
			}
		})
	}
}

// Settled delivered, the report is not sent and what it answered counts as
// answered.
func TestAnEarlierReportSettledDeliveredIsNotSent(t *testing.T) {
	dir, self, web := toolSession(t)
	earlier := earlierTask(t, dir, web)
	report := writeEarlierBuildReceipt(t, dir, web, earlier, false)
	if completeTurn(dir, self, turnResult{Boundary: boundaryNow(t, dir, self), ID: "second", Text: "done"}, "") == nil {
		t.Fatal("not stopped")
	}
	if code, _, errOut := asPerson(t, "settle", "api", report.ID, "--delivered", "--json"); code != ExitOK {
		t.Fatalf("settle: %d %s", code, errOut)
	}
	if answersTo(t, dir, earlier) != 0 || earlierOwes(t, dir, earlier) {
		t.Fatal("a report settled delivered went, or its task stayed owed")
	}
	for _, words := range [][]string{
		{"settle", "api", report.ID},
		{"settle", "api", report.ID, "--delivered", "--undelivered"},
		{"settle", "api", inbox.NewID(), "--delivered"},
	} {
		if code, _, _ := asPerson(t, words...); code != ExitUsage {
			t.Errorf("%v: %d, want a refusal", words, code)
		}
	}
}

// The earlier build's kept answer belongs to its ended run: never taken, it is
// not carried into a report of this build's run.
func TestAnEarlierBuildsKeptAnswerIsNeverTaken(t *testing.T) {
	dir, self, web := toolSession(t)
	pending := filepath.Join(state.InboxPath(dir, "api"), "pending")
	if err := state.EnsureSubdir(pending); err != nil {
		t.Fatal(err)
	}
	record := fmt.Sprintf(`{"epoch":%q,"text":%q}`, earlierAPIRun, "HELD_BY_THE_EARLIER_END")
	if err := os.WriteFile(filepath.Join(pending, "kept.json"), []byte(record), 0o600); err != nil {
		t.Fatal(err)
	}
	fresh := readKind(t, dir, web, inbox.Question)
	if err := completeTurn(dir, self, turnResult{Boundary: boundaryNow(t, dir, self), ID: "second", Text: "FRESH_ANSWER"}, ""); err != nil {
		t.Fatal(err)
	}
	for _, report := range reportsTo(t, dir, "web") {
		if len(report.InReplyTo) == 1 && report.InReplyTo[0] == fresh && strings.Contains(report.Text, "HELD_BY_THE_EARLIER_END") {
			t.Fatalf("an ended run's kept answer was taken: %q", report.Text)
		}
	}
}

// The stop outlives the sweep: days later the conversion journal still names
// the report, and settling it still sends it once.
func TestAnEarlierBuildsReceiptOutlivesTheSweep(t *testing.T) {
	dir, self, web := toolSession(t)
	earlier := earlierTask(t, dir, web)
	report := writeEarlierBuildReceipt(t, dir, web, earlier, false)
	for range 2 {
		aDayPasses(t, dir)
		sweepAsServer(t, dir, self)
	}
	if inbox.MailboxStopped(dir, "api") == nil {
		t.Fatal("the sweep let the stop go")
	}
	if code, _, errOut := asPerson(t, "settle", "api", report.ID, "--undelivered"); code != ExitOK {
		t.Fatalf("settle: %d %s", code, errOut)
	}
	if answersTo(t, dir, earlier) != 1 {
		t.Fatalf("the settled report went %d times", answersTo(t, dir, earlier))
	}
}

// One not prepared recorded no scope, so it answers nothing that is owed now.
func TestAnEarlierBuildsUnpreparedReceiptAnswersNothing(t *testing.T) {
	dir, self, web := toolSession(t)
	earlier := earlierTask(t, dir, web)
	writeReceiptFile(t, dir, earlierBuildReceipt{ID: inbox.NewID()})
	fresh := readKind(t, dir, web, inbox.Question)
	if err := completeTurn(dir, self, turnResult{Boundary: boundaryNow(t, dir, self), ID: "second", Text: "done"}, ""); err != nil {
		t.Fatal(err)
	}
	if answersTo(t, dir, earlier) != 0 || !earlierOwes(t, dir, earlier) || answersTo(t, dir, fresh) != 1 {
		t.Fatalf("the unprepared receipt answered: earlier %d, fresh %d", answersTo(t, dir, earlier), answersTo(t, dir, fresh))
	}
}
