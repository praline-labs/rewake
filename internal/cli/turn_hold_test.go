package cli

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/harness/claude/telemetry"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
)

// stopPayload is a Claude Code Stop hook's stdin: the first call of a turn
// end, or with active the call after a hold.
func stopPayload(t *testing.T, event, text string, active bool) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"hook_event_name": event, "session_id": "t", "last_assistant_message": text, "stop_hook_active": active,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// interimLab is api after a turn that ended pending: web's task is read and
// owed, web has the interim message, and the next turn has started at start.
type interimLab struct {
	dir   string
	self  registry.Session
	start int64
}

// interimLab's times are on the boot clock below this test's own start, which
// is when a hook run in-process ends its turn.
func newInterimLab(t *testing.T) interimLab {
	t.Helper()
	dir := liveSession(t, "api")
	peer := otherRun(t, dir, "web")
	readFrom(t, dir, peer)
	self, _ := registry.Lookup(dir, "api")
	if err := inbox.MarkPending(dir, "api", self.Epoch(), "the suite is running", markAt-8); err != nil {
		t.Fatal(err)
	}
	if err := completeTurn(dir, self, turnResult{Text: "started the suite", Started: markAt - 10, Ended: markAt - 5}, "t"); err != nil {
		t.Fatal(err)
	}
	if got := kinds(reportsTo(t, dir, "web")); len(got) != 1 || got[0] != string(inbox.Interim) {
		t.Fatalf("the first turn end gave web %v, want one interim message", got)
	}
	// What turn-ended records at the end of the interim turn.
	turnStarted(t, dir, self, markAt-5)
	return interimLab{dir: dir, self: self, start: markAt - 5}
}

func (lab interimLab) owed() int { return len(inbox.Waiters(lab.dir, "api", lab.self.Epoch())) }

func (lab interimLab) turnStart() int64 {
	return telemetry.ReadTurnStart(telemetry.TurnStartPath(registry.ObservationFor(lab.dir, lab.self.Name, lab.self.Epoch())))
}

// held runs a first Stop that must be held, and answers the reason.
func (lab interimLab) held(t *testing.T, text string) string {
	t.Helper()
	code, out, errOut := run("turn-ended", stopPayload(t, "Stop", text, false))
	var decision struct{ Decision, Reason string }
	if code != ExitOK || json.Unmarshal([]byte(out), &decision) != nil || decision.Decision != "block" {
		t.Fatalf("the unmarked turn end after an interim one was not held: %d %q %s", code, out, errOut)
	}
	return decision.Reason
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
	if got := lab.turnStart(); got != lab.start {
		t.Errorf("the hold moved the turn's start from %d to %d", lab.start, got)
	}

	code, out, _ := run("turn-ended", stopPayload(t, "Stop", "nothing more to add", true))
	if code != ExitOK || out != "" {
		t.Fatalf("the call after the hold printed %q", out)
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
	if _, kept := inbox.KeptAnswer(lab.dir, "api", lab.self.Epoch()); kept {
		t.Error("the kept answer outlived its report")
	}
	if _, interim := inbox.LastInterim(lab.dir, "api", lab.self.Epoch()); interim {
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
	run("turn-ended", stopPayload(t, "Stop", "marked", true))
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
	if line, _ := inbox.LastInterim(lab.dir, "api", lab.self.Epoch()); line != "the second half is running" {
		t.Errorf("the run is interim with %q", line)
	}
	if reason := lab.held(t, "all green"); !strings.Contains(reason, "the second half is running") {
		t.Errorf("the next hold asks about %s", reason)
	}
}

// An Esc during the continuation ends it with no Stop: the plugin's stop takes
// the held answer, after its own line, and keeps the task owed.
func TestAStopAfterAHoldCarriesTheHeldAnswer(t *testing.T) {
	lab := newInterimLab(t)
	lab.held(t, "the report")
	if err := completeTurn(lab.dir, lab.self, turnResult{ID: "claude/turn-2", Stopped: true, Text: telemetry.StoppedText, Started: lab.start, Ended: markAt + 1}, "t"); err != nil {
		t.Fatal(err)
	}
	var stopped []string
	for _, report := range reportsTo(t, lab.dir, "web") {
		if inbox.KindOf(report) == inbox.Stopped {
			stopped = append(stopped, report.Text)
		}
	}
	if len(stopped) != 1 || stopped[0] != telemetry.StoppedText+"\n\nthe report" || lab.owed() != 1 {
		t.Fatalf("the stop after a hold sent %q, %d owed", stopped, lab.owed())
	}
	if _, kept := inbox.KeptAnswer(lab.dir, "api", lab.self.Epoch()); kept {
		t.Error("the stop left the answer kept")
	}
	if _, interim := inbox.LastInterim(lab.dir, "api", lab.self.Epoch()); !interim {
		t.Error("a stop cleared the interim record; the work is as it was")
	}
}

// A failure of the continuation leads with its reason, and the held answer
// follows it.
func TestAFailureAfterAHoldCarriesTheHeldAnswer(t *testing.T) {
	lab := newInterimLab(t)
	lab.held(t, "the report")
	run("turn-ended", stopPayload(t, "StopFailure", "rate limited", true))
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
	code, out, _ := run("turn-ended", stopPayload(t, "Stop", "the next answer", false))
	if code != ExitOK || out != "" {
		t.Fatalf("a second hold with an answer kept: %q", out)
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
		payload string
	}{
		{"after a report", func(t *testing.T, lab interimLab) {
			if err := inbox.ClearInterim(lab.dir, "api"); err != nil {
				t.Fatal(err)
			}
		}, `{"hook_event_name":"Stop","session_id":"t","last_assistant_message":"done","stop_hook_active":false}`},
		{"marked in this turn", func(t *testing.T, lab interimLab) {
			if err := inbox.MarkPending(lab.dir, "api", lab.self.Epoch(), "still going", markAt-1); err != nil {
				t.Fatal(err)
			}
		}, `{"hook_event_name":"Stop","session_id":"t","last_assistant_message":"done","stop_hook_active":false}`},
		{"a failure", nil, `{"hook_event_name":"StopFailure","session_id":"t","last_assistant_message":"broke","error":"api_error"}`},
		{"after another hook's hold", nil, `{"hook_event_name":"Stop","session_id":"t","last_assistant_message":"done","stop_hook_active":true}`},
		{"without stop_hook_active", nil, `{"hook_event_name":"Stop","session_id":"t","last_assistant_message":"done"}`},
		{"Codex", nil, `{"type":"agent-turn-complete","turn-id":"x","last-assistant-message":"done"}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			lab := newInterimLab(t)
			if c.prepare != nil {
				c.prepare(t, lab)
			}
			if code, out, _ := run("turn-ended", c.payload); code != ExitOK || out != "" {
				t.Fatalf("held: %q", out)
			}
			if got := kinds(reportsTo(t, lab.dir, "web")); len(got) != 2 {
				t.Errorf("web holds %v, want the interim message and this end's", got)
			}
		})
	}
	t.Run("nobody waiting", func(t *testing.T) {
		lab := newInterimLab(t)
		for _, waiter := range inbox.Waiters(lab.dir, "api", lab.self.Epoch()) {
			inbox.ClearAwaiting(lab.dir, "api", lab.self.Epoch(), waiter)
		}
		if _, out, _ := run("turn-ended", stopPayload(t, "Stop", "done", false)); out != "" {
			t.Fatalf("held with nobody waiting: %q", out)
		}
	})
}
