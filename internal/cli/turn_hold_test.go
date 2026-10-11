package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// stopEnd is a turn end from a harness that lets an end be held once: the
// first end of a turn, or with again the end after a hold, which is not held.
func stopEnd(text string, again bool) boundaryEnd {
	return boundaryEnd{text: text, holdable: !again, thread: "t"}
}

// stoppedText is what a harness's end says of a turn a person stopped.
const stoppedText = "the person at the keyboard stopped this turn"

// interimLab is api after a turn that ended pending: web's task is read and
// owed, web has the interim message, and the next turn has started at start.
type interimLab struct {
	dir   string
	self  registry.Session
	start int64
}

// interimLab's times are on the boot clock below this test's own start; a
// turn the test ends afterwards ends now.
func newInterimLab(t *testing.T) interimLab {
	t.Helper()
	dir := liveSession(t, "api")
	peer := otherRun(t, dir, "web")
	readFrom(t, dir, peer)
	self, _ := registry.Lookup(dir, "api")
	if err := markPending(dir, "api", self.Epoch(), "the suite is running", markAt-8); err != nil {
		t.Fatal(err)
	}
	if err := completeTurn(dir, self, inbox.TurnEnd{Text: "started the suite", Started: markAt - 10, Ended: markAt - 5}, "t"); err != nil {
		t.Fatal(err)
	}
	if got := kinds(reportsTo(t, dir, "web")); len(got) != 1 || got[0] != string(inbox.Interim) {
		t.Fatalf("the first turn end gave web %v, want one interim message", got)
	}
	// The next turn starts as the interim one ends.
	turnStarted(t, dir, self, markAt-5)
	return interimLab{dir: dir, self: self, start: markAt - 5}
}

func (lab interimLab) owed() int { return len(inbox.Waiters(lab.dir, "api", lab.self.Epoch())) }

func (lab interimLab) turnStart(t *testing.T) int64 { return recordedStart(t, lab.dir, lab.self) }

// held runs a first end that must be held, and answers the reason.
func (lab interimLab) held(t *testing.T, text string) string {
	t.Helper()
	reason, err := endTurn(t, stopEnd(text, false))
	if err != nil || reason == "" {
		t.Fatalf("the unmarked turn end after an interim one was not held: %q %v", reason, err)
	}
	return reason
}

// The unmarked turn end after an interim one is held once and publishes
// nothing; the call after the hold is not held, and its report carries the
// held answer first and the continuation after it.
func TestAnUnmarkedEndAfterAnInterimOneIsHeldOnce(t *testing.T) {
	lab := newInterimLab(t)
	reason := lab.held(t, "the suite is green: 40 pass")
	for _, want := range []string{`ended pending ("the suite is running")`, "web still wait", "rewake pending", "do not repeat it"} {
		if !strings.Contains(reason, want) {
			t.Errorf("the reason lacks %q: %s", want, reason)
		}
	}
	if got := kinds(reportsTo(t, lab.dir, "web")); len(got) != 1 || lab.owed() != 1 {
		t.Fatalf("the hold published %v and left %d owed", got, lab.owed())
	}
	// A mark the continuation makes must fall inside the turn: the hold is
	// not an end, and records no start.
	if got := lab.turnStart(t); got != lab.start {
		t.Errorf("the hold moved the turn's start from %d to %d", lab.start, got)
	}

	if reason, err := endTurn(t, stopEnd("nothing more to add", true)); err != nil || reason != "" {
		t.Fatalf("the end after the hold: %q %v", reason, err)
	}
	reports := reportsTo(t, lab.dir, "web")
	var report *inbox.Message
	for i := range reports {
		if inbox.KindOf(reports[i]) == inbox.Finished {
			report = &reports[i]
		}
	}
	if report == nil || report.Text != "the suite is green: 40 pass\n\nnothing more to add" || lab.owed() != 0 {
		t.Fatalf("after the continuation web holds %+v, %d owed", reports, lab.owed())
	}
	if _, kept, _ := inbox.KeptAnswer(lab.dir, "api", lab.self.Epoch()); kept {
		t.Error("the kept answer outlived its report")
	}
	if _, interim, _ := inbox.LastInterim(lab.dir, "api", lab.self.Epoch()); interim {
		t.Error("the report left the run marked interim")
	}
}

// A continuation that marks pending ends interim with the mark's line, then the
// held answer, then its own text; the task stays owed, and the next unmarked
// end is asked again, about the new line.
func TestAContinuationThatMarksPendingKeepsTheTaskOwed(t *testing.T) {
	lab := newInterimLab(t)
	lab.held(t, "half of the suite passed")
	if code, _, errOut := run("pending", "the second half is running"); code != ExitOK {
		t.Fatalf("pending: %s", errOut)
	}
	if _, err := endTurn(t, stopEnd("marked", true)); err != nil {
		t.Fatal(err)
	}
	reports := reportsTo(t, lab.dir, "web")
	var texts []string
	for _, report := range reports {
		if inbox.KindOf(report) == inbox.Interim {
			texts = append(texts, report.Text)
		}
	}
	if len(reports) != 2 || len(texts) != 2 || !slices.Contains(texts, "the second half is running\n\nhalf of the suite passed\n\nmarked") || lab.owed() != 1 {
		t.Fatalf("after the marked continuation web holds %+v, %d owed", reports, lab.owed())
	}
	if line, _, _ := inbox.LastInterim(lab.dir, "api", lab.self.Epoch()); line != "the second half is running" {
		t.Errorf("the run is interim with %q", line)
	}
	if reason := lab.held(t, "all green"); !strings.Contains(reason, "the second half is running") {
		t.Errorf("the next hold asks about %s", reason)
	}
}

// A person who stops the continuation ends it stopped: the stop takes the held
// answer, after its own line, and keeps the task owed.
func TestAStopAfterAHoldCarriesTheHeldAnswer(t *testing.T) {
	lab := newInterimLab(t)
	lab.held(t, "the report")
	if err := completeTurn(lab.dir, lab.self, inbox.TurnEnd{Boundary: boundaryNow(t, lab.dir, lab.self), ID: "turn-2", Stopped: true, Text: stoppedText, Started: lab.start, Ended: markAt + 1}, "t"); err != nil {
		t.Fatal(err)
	}
	var stopped []string
	for _, report := range reportsTo(t, lab.dir, "web") {
		if inbox.KindOf(report) == inbox.Stopped {
			stopped = append(stopped, report.Text)
		}
	}
	if len(stopped) != 1 || stopped[0] != stoppedText+"\n\nthe report" || lab.owed() != 1 {
		t.Fatalf("the stop after a hold sent %q, %d owed", stopped, lab.owed())
	}
	if _, kept, _ := inbox.KeptAnswer(lab.dir, "api", lab.self.Epoch()); kept {
		t.Error("the stop left the answer kept")
	}
	if _, interim, _ := inbox.LastInterim(lab.dir, "api", lab.self.Epoch()); !interim {
		t.Error("a stop cleared the interim record; the work is as it was")
	}
}

// A failure of the continuation leads with its reason, and the held answer
// follows it.
func TestAFailureAfterAHoldCarriesTheHeldAnswer(t *testing.T) {
	lab := newInterimLab(t)
	lab.held(t, "the report")
	if _, err := endTurn(t, boundaryEnd{text: "rate limited", failed: true, thread: "t"}); err != nil {
		t.Fatal(err)
	}
	var failures []string
	for _, report := range reportsTo(t, lab.dir, "web") {
		if inbox.KindOf(report) == inbox.Error {
			failures = append(failures, report.Text)
		}
	}
	if len(failures) != 1 || failures[0] != "rate limited\n\nthe report" {
		t.Fatalf("the failure after a hold sent %q", failures)
	}
}

// A held answer whose continuation was never heard goes out with the next turn
// end, which is not held again.
func TestAnAnswerHeldAndNeverContinuedGoesWithTheNextEnd(t *testing.T) {
	lab := newInterimLab(t)
	lab.held(t, "the lost report")
	if reason, err := endTurn(t, stopEnd("the next answer", false)); err != nil || reason != "" {
		t.Fatalf("a second hold with an answer kept: %q %v", reason, err)
	}
	for _, report := range reportsTo(t, lab.dir, "web") {
		if inbox.KindOf(report) == inbox.Finished && report.Text == "the lost report\n\nthe next answer" {
			return
		}
	}
	t.Fatalf("web holds %+v", reportsTo(t, lab.dir, "web"))
}

// Every other turn end is published as it was: no hold without an interim end
// before it, with a mark in this turn, with nobody waiting, on a failure, on
// the call after a hold, or from a harness that cannot hold.
func TestATurnEndIsHeldOnlyAfterAnInterimOne(t *testing.T) {
	for _, c := range []struct {
		name    string
		prepare func(t *testing.T, lab interimLab)
		end     boundaryEnd
	}{
		{"after a report", func(t *testing.T, lab interimLab) {
			if err := os.Remove(filepath.Join(state.InboxPath(lab.dir, "api"), "pending", "interim.json")); err != nil {
				t.Fatal(err)
			}
		}, stopEnd("done", false)},
		{"marked in this turn", func(t *testing.T, lab interimLab) {
			if err := markPending(lab.dir, "api", lab.self.Epoch(), "still going", markAt-1); err != nil {
				t.Fatal(err)
			}
		}, stopEnd("done", false)},
		{"a failure", nil, boundaryEnd{text: "broke", failed: true, holdable: true, thread: "t"}},
		{"after a hold", nil, stopEnd("done", true)},
		{"from a harness that cannot hold", nil, boundaryEnd{text: "done", thread: "t"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			lab := newInterimLab(t)
			if c.prepare != nil {
				c.prepare(t, lab)
			}
			if reason, err := endTurn(t, c.end); err != nil || reason != "" {
				t.Fatalf("held: %q %v", reason, err)
			}
			if got := kinds(reportsTo(t, lab.dir, "web")); len(got) != 2 {
				t.Errorf("web holds %v, want the interim message and this end's", got)
			}
		})
	}
	t.Run("nobody waiting", func(t *testing.T) {
		lab := newInterimLab(t)
		for _, waiter := range inbox.Waiters(lab.dir, "api", lab.self.Epoch()) {
			if err := inbox.ClearAwaiting(lab.dir, "api", lab.self.Epoch(), waiter); err != nil {
				t.Fatal(err)
			}
		}
		if reason, _ := endTurn(t, stopEnd("done", false)); reason != "" {
			t.Fatalf("held with nobody waiting: %q", reason)
		}
	})
}
