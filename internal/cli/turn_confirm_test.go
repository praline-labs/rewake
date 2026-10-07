package cli

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/state"
)

// The oracles of turn_hold_test.go, written again against the neutral
// confirmation an adapter that can hold an end calls (ConfirmCompletion):
// the same rules, reached without a Stop hook.

// end is a turn end of the interim lab's session, as an adapter reports it:
// its event and its moment on the boot clock, after the lab's turn start.
func (lab interimLab) end(id string, at int64, kind inbox.Kind, text string) harness.Completion {
	return harness.Completion{ID: id, Kind: kind, Text: text, Started: lab.start, Ended: markAt + at}
}

// confirm confirms an end with the boundary the run's clock gives now.
func (lab interimLab) confirm(t *testing.T, end harness.Completion) string {
	t.Helper()
	end.Boundary = boundaryNow(t, lab.dir, lab.self)
	reason, err := ConfirmCompletion(context.Background(), lab.dir, lab.self, end)
	if err != nil {
		t.Fatal(err)
	}
	return reason
}

// reportsOf answers the texts of web's reports of one kind.
func (lab interimLab) reportsOf(t *testing.T, kind inbox.Kind) []string {
	t.Helper()
	var texts []string
	for _, report := range reportsTo(t, lab.dir, "web") {
		if inbox.KindOf(report) == kind {
			texts = append(texts, report.Text)
		}
	}
	return texts
}

func (lab interimLab) kept() bool {
	_, kept, _ := inbox.KeptAnswer(lab.dir, "api", lab.self.Epoch())
	return kept
}

func TestAConfirmedEndAfterAnInterimOneIsHeldOnce(t *testing.T) {
	lab := newInterimLab(t)
	reason := lab.confirm(t, lab.end("turn-2", 1, inbox.Finished, "the suite is green: 40 pass"))
	for _, want := range []string{`ended pending ("the suite is running")`, "web still wait", "rewake pending", "do not repeat it"} {
		if !strings.Contains(reason, want) {
			t.Errorf("the reason lacks %q: %s", want, reason)
		}
	}
	if got := kinds(reportsTo(t, lab.dir, "web")); len(got) != 1 || lab.owed() != 1 {
		t.Fatalf("the hold published %v and left %d owed", got, lab.owed())
	}
	if reason := lab.confirm(t, lab.end("turn-2b", 2, inbox.Finished, "nothing more to add")); reason != "" {
		t.Fatalf("the continuation was held again: %s", reason)
	}
	if got := lab.reportsOf(t, inbox.Finished); len(got) != 1 || got[0] != "the suite is green: 40 pass\n\nnothing more to add" || lab.owed() != 0 {
		t.Fatalf("after the continuation web holds %q, %d owed", got, lab.owed())
	}
	if lab.kept() {
		t.Error("the kept answer outlived its report")
	}
	if _, interim, _ := inbox.LastInterim(lab.dir, "api", lab.self.Epoch()); interim {
		t.Error("the report left the run marked interim")
	}
}

func TestAConfirmedContinuationThatMarksPendingKeepsTheTaskOwed(t *testing.T) {
	lab := newInterimLab(t)
	lab.confirm(t, lab.end("turn-2", 1, inbox.Finished, "half of the suite passed"))
	if code, _, errOut := run("pending", "the second half is running"); code != ExitOK {
		t.Fatalf("pending: %s", errOut)
	}
	lab.confirm(t, lab.end("turn-2b", 2, inbox.Finished, "marked"))
	texts := lab.reportsOf(t, inbox.Interim)
	if len(reportsTo(t, lab.dir, "web")) != 2 || !slices.Contains(texts, "the second half is running\n\nhalf of the suite passed\n\nmarked") || lab.owed() != 1 {
		t.Fatalf("after the marked continuation web holds %q, %d owed", texts, lab.owed())
	}
	if reason := lab.confirm(t, lab.end("turn-3", 3, inbox.Finished, "all green")); !strings.Contains(reason, "the second half is running") {
		t.Errorf("the next hold asks about %q", reason)
	}
}

// A failed and an interrupted continuation are never held, and each takes the
// kept answer after its own line.
func TestAFailedOrStoppedContinuationCarriesTheHeldAnswer(t *testing.T) {
	for _, kind := range []inbox.Kind{inbox.Error, inbox.Stopped} {
		t.Run(string(kind), func(t *testing.T) {
			lab := newInterimLab(t)
			lab.confirm(t, lab.end("turn-2", 1, inbox.Finished, "the report"))
			if reason := lab.confirm(t, lab.end("turn-2b", 2, kind, "cut short")); reason != "" {
				t.Fatalf("a %s continuation was held: %s", kind, reason)
			}
			if got := lab.reportsOf(t, kind); len(got) != 1 || got[0] != "cut short\n\nthe report" {
				t.Fatalf("the %s after a hold sent %q", kind, got)
			}
			if lab.kept() {
				t.Error("the continuation left the answer kept")
			}
		})
	}
}

func TestAnAnswerHeldAndNeverContinuedGoesWithTheNextConfirmedEnd(t *testing.T) {
	lab := newInterimLab(t)
	lab.confirm(t, lab.end("turn-2", 1, inbox.Finished, "the lost report"))
	if reason := lab.confirm(t, lab.end("turn-3", 2, inbox.Finished, "the next answer")); reason != "" {
		t.Fatalf("a second hold with an answer kept: %q", reason)
	}
	if got := lab.reportsOf(t, inbox.Finished); len(got) != 1 || got[0] != "the lost report\n\nthe next answer" {
		t.Fatalf("web holds %q", got)
	}
}

// Every other end is published as it was: no hold without an interim end
// before it, with a mark in this turn, with nobody waiting, on a failure or a
// stop, or through Publish, which an adapter that cannot hold calls.
func TestAConfirmedEndIsHeldOnlyAfterAnInterimOne(t *testing.T) {
	for _, c := range []struct {
		name    string
		prepare func(t *testing.T, lab interimLab)
		kind    inbox.Kind
	}{
		{"after a report", func(t *testing.T, lab interimLab) {
			if err := os.Remove(filepath.Join(state.InboxPath(lab.dir, "api"), "pending", "interim.json")); err != nil {
				t.Fatal(err)
			}
		}, inbox.Finished},
		{"marked in this turn", func(t *testing.T, lab interimLab) {
			if err := markPending(lab.dir, "api", lab.self.Epoch(), "still going", markAt-1); err != nil {
				t.Fatal(err)
			}
		}, inbox.Finished},
		{"nobody waiting", func(t *testing.T, lab interimLab) {
			for _, waiter := range inbox.Waiters(lab.dir, "api", lab.self.Epoch()) {
				if err := inbox.ClearAwaiting(lab.dir, "api", lab.self.Epoch(), waiter); err != nil {
					t.Fatal(err)
				}
			}
		}, inbox.Finished},
		{"a failure", nil, inbox.Error},
		{"a stop", nil, inbox.Stopped},
	} {
		t.Run(c.name, func(t *testing.T) {
			lab := newInterimLab(t)
			if c.prepare != nil {
				c.prepare(t, lab)
			}
			before := len(reportsTo(t, lab.dir, "web"))
			if reason := lab.confirm(t, lab.end("turn-2", 1, c.kind, "done")); reason != "" {
				t.Fatalf("held: %q", reason)
			}
			if lab.kept() {
				t.Error("an end not held kept its answer")
			}
			if c.name != "nobody waiting" && len(reportsTo(t, lab.dir, "web")) != before+1 {
				t.Errorf("web holds %v, want the interim message and this end's", kinds(reportsTo(t, lab.dir, "web")))
			}
		})
	}
	t.Run("through Publish", func(t *testing.T) {
		lab := newInterimLab(t)
		end := lab.end("turn-2", 1, inbox.Finished, "done")
		end.Boundary = boundaryNow(t, lab.dir, lab.self)
		if err := ReportCompletion(context.Background(), lab.dir, lab.self, end); err != nil {
			t.Fatal(err)
		}
		if got := kinds(reportsTo(t, lab.dir, "web")); len(got) != 2 || lab.kept() {
			t.Errorf("Publish gave web %v, kept %v", got, lab.kept())
		}
	})
}

// O1: every read and write of the check failing falls to publishing, the
// behavior without the hold, and keeps nothing. The check's own access is the
// nth of its path: before the check, the barrier reads the interim record and
// the lookup of an end on record reads the kept answer, and a failure there
// stops the end instead, as an unreadable record does.
func TestEveryFailureOfTheConfirmationCheckFallsToPublishing(t *testing.T) {
	for _, c := range []struct {
		name, op, suffix string
		nth              int
	}{
		{"the interim record", state.OpRead, filepath.Join("pending", "interim.json"), 2},
		{"the kept answer", state.OpRead, filepath.Join("pending", "kept.json"), 2},
		{"the kept answer before keeping", state.OpRead, filepath.Join("pending", "kept.json"), 3},
		{"the sender's record", state.OpRead, filepath.Join("sessions", "web.json"), 1},
		{"the clock's reservation", state.OpWrite, ".read-high", 1},
		{"keeping the answer", state.OpWrite, filepath.Join("pending", "kept.json"), 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			lab := newInterimLab(t)
			seen, failed := 0, false
			state.Fault = func(op, path string) error {
				if op != c.op || !strings.HasSuffix(path, c.suffix) {
					return nil
				}
				if seen++; seen == c.nth {
					failed = true
					return os.ErrPermission
				}
				return nil
			}
			t.Cleanup(func() { state.Fault = nil })
			if reason := lab.confirm(t, lab.end("turn-2", 1, inbox.Finished, "done")); reason != "" {
				t.Fatalf("held despite the failure: %q", reason)
			}
			state.Fault = nil
			if !failed {
				t.Fatalf("nothing asked to %s %s", c.op, c.suffix)
			}
			if got := lab.reportsOf(t, inbox.Finished); len(got) != 1 || got[0] != "done" || lab.owed() != 0 || lab.kept() {
				t.Fatalf("after the failure web holds %q, %d owed, kept %v", got, lab.owed(), lab.kept())
			}
		})
	}
}

// An end that names no event, or carries no boundary, is refused: a held end
// confirmed again could not be known. Its waits stay owed.
func TestAConfirmedEndWithoutItsEventIsRefused(t *testing.T) {
	lab := newInterimLab(t)
	for _, end := range []harness.Completion{
		{Kind: inbox.Finished, Text: "done", Boundary: boundaryNow(t, lab.dir, lab.self), Started: lab.start, Ended: markAt + 1},
		lab.end("turn-2", 1, inbox.Finished, "done"),
	} {
		if reason, err := ConfirmCompletion(context.Background(), lab.dir, lab.self, end); err == nil || reason != "" {
			t.Fatalf("confirmed %+v: %q %v", end, reason, err)
		}
	}
	if got := kinds(reportsTo(t, lab.dir, "web")); len(got) != 1 || lab.owed() != 1 || lab.kept() {
		t.Fatalf("the refused ends gave web %v, %d owed, kept %v", got, lab.owed(), lab.kept())
	}
}
