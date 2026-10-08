package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/boottime"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

func reportsTo(t *testing.T, dir, name string) []inbox.Message {
	t.Helper()
	var messages []inbox.Message
	for _, file := range finishedFor(t, dir, name) {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var message inbox.Message
		if err := json.Unmarshal(raw, &message); err != nil {
			t.Fatal(err)
		}
		messages = append(messages, message)
	}
	return messages
}

func kinds(messages []inbox.Message) []string {
	var out []string
	for _, message := range messages {
		out = append(out, string(inbox.KindOf(message)))
	}
	return out
}

// turnStarted records the start of the turn now running, the way the host
// does when it hears one: at the given time on the boot clock, in the core.
func turnStarted(t *testing.T, dir string, self registry.Session, at int64) {
	t.Helper()
	if err := inbox.RecordTurnStart(dir, self.Name, self.Epoch(), at); err != nil {
		t.Fatal(err)
	}
}

// recordedStart is the latest turn start the run has on record.
func recordedStart(t *testing.T, dir string, self registry.Session) int64 {
	t.Helper()
	started, err := latestTurnStart(dir, self, self.Epoch())
	if err != nil {
		t.Fatal(err)
	}
	return started
}

// markAt is the time this test process's `rewake pending` marks carry: the
// command runs in-process, so its process start is the test binary's.
var markAt = boottime.ProcessStarted

// A turn end marked pending tells the sender the work is still going and
// keeps the task owed; the next turn end without a mark is the report.
func TestAPendingTurnEndKeepsTheTaskOwed(t *testing.T) {
	dir := liveSession(t, "api")
	peer := otherRun(t, dir, "web")
	task := readFrom(t, dir, peer)
	self, _ := registry.Lookup(dir, "api")
	turnStarted(t, dir, self, markAt-1)

	if code, out, errOut := run("pending", "the suite is running"); code != ExitOK || out != "Rewake: marked pending; at this turn's end web will read that the work goes on.\n" {
		t.Fatalf("pending: %d %s %s", code, out, errOut)
	}
	end := boottime.Now()
	if err := completeTurn(dir, self, inbox.TurnEnd{Boundary: boundaryNow(t, dir, self), ID: "t/1", Text: "waiting for the suite", Started: markAt - 1, Ended: end}, "t"); err != nil {
		t.Fatal(err)
	}
	interim := reportsTo(t, dir, "web")
	// The mark's line leads, as the preview; the turn's own text follows it.
	if len(interim) != 1 || inbox.KindOf(interim[0]) != inbox.Interim || interim[0].Text != "the suite is running\n\nwaiting for the suite" ||
		len(interim[0].InReplyTo) != 1 || interim[0].InReplyTo[0] != task || inbox.Owed(interim[0]) {
		t.Fatalf("after the pending turn end web holds %+v", interim)
	}
	if len(inbox.Waiters(dir, "api", self.Epoch())) != 1 {
		t.Fatal("the pending turn end settled the task")
	}
	// A retry of the same turn does not change its outcome.
	if err := completeTurn(dir, self, inbox.TurnEnd{Boundary: boundaryNow(t, dir, self), ID: "t/1", Text: "waiting for the suite", Started: markAt - 1, Ended: end}, "t"); err != nil {
		t.Fatal(err)
	}
	if got := kinds(reportsTo(t, dir, "web")); len(got) != 1 {
		t.Fatalf("a retried pending turn wrote %v", got)
	}

	next := boottime.Now()
	if err := completeTurn(dir, self, inbox.TurnEnd{Boundary: boundaryNow(t, dir, self), ID: "t/2", Text: "the suite is green", Started: next, Ended: boottime.Now()}, "t"); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(kinds(reportsTo(t, dir, "web")), ",")
	if got != "pending,finished" && got != "finished,pending" {
		t.Fatalf("web holds %v, want the interim message and the report", got)
	}
	if len(inbox.Waiters(dir, "api", self.Epoch())) != 0 {
		t.Fatal("the report did not settle the task")
	}
}

// Pending, then an interruption that ends the turn without rewake hearing of
// it, then a new turn that ends: that is the report, not an interim message.
func TestAMarkDoesNotSurviveAnInterruptedTurn(t *testing.T) {
	dir := liveSession(t, "api")
	peer := otherRun(t, dir, "web")
	readFrom(t, dir, peer)
	self, _ := registry.Lookup(dir, "api")
	turnStarted(t, dir, self, markAt-1)
	if code, _, errOut := run("pending", "waiting"); code != ExitOK {
		t.Fatalf("pending: %s", errOut)
	}
	// Esc: no turn end. The person types "go on": a new turn starts.
	next := boottime.Now()
	turnStarted(t, dir, self, next)
	if err := completeTurn(dir, self, inbox.TurnEnd{Boundary: boundaryNow(t, dir, self), ID: "t/2", Text: "the final answer", Started: next, Ended: boottime.Now()}, "t"); err != nil {
		t.Fatal(err)
	}
	got := reportsTo(t, dir, "web")
	if len(got) != 1 || inbox.KindOf(got[0]) != inbox.Finished || got[0].Text != "the final answer" {
		t.Fatalf("web holds %v, want the final answer as the report", kinds(got))
	}
	if len(inbox.Waiters(dir, "api", self.Epoch())) != 0 {
		t.Fatal("the report did not settle the task")
	}
}

// Codex publishes turn K late. A mark made in K+1 meanwhile belongs to K+1:
// K is a report with its own text, and K+1 is the interim turn end.
func TestALatePublishedTurnIsAReportWithItsOwnText(t *testing.T) {
	dir := liveSession(t, "api")
	peer := otherRun(t, dir, "web")
	readFrom(t, dir, peer)
	self, _ := registry.Lookup(dir, "api")
	turnStarted(t, dir, self, markAt-1)
	if code, _, errOut := run("pending", "K+1 waits for the suite"); code != ExitOK {
		t.Fatalf("pending: %s", errOut)
	}
	// Turn K ran and ended before the mark; its end is published only now.
	if err := completeTurn(dir, self, inbox.TurnEnd{Boundary: boundaryNow(t, dir, self), ID: "t/K", Text: "K's own answer", Started: markAt - 10, Ended: markAt - 5}, "t"); err != nil {
		t.Fatal(err)
	}
	got := reportsTo(t, dir, "web")
	if len(got) != 1 || inbox.KindOf(got[0]) != inbox.Finished || got[0].Text != "K's own answer" {
		t.Fatalf("after K web holds %v, want K's own report", kinds(got))
	}
	// A second task keeps K+1 owing something, so its interim end has a waiter.
	readFrom(t, dir, peer)
	if err := completeTurn(dir, self, inbox.TurnEnd{Boundary: boundaryNow(t, dir, self), ID: "t/K+1", Text: "still going", Started: markAt - 1, Ended: boottime.Now()}, "t"); err != nil {
		t.Fatal(err)
	}
	var interim bool
	for _, message := range reportsTo(t, dir, "web") {
		interim = interim || inbox.KindOf(message) == inbox.Interim && message.Text == "K+1 waits for the suite\n\nstill going"
	}
	if !interim {
		t.Fatalf("K+1 did not end as the interim turn end: %v", kinds(reportsTo(t, dir, "web")))
	}
}

// A pending turn end whose turn said nothing carries the mark's line alone.
func TestAPendingTurnEndWithoutTextIsItsMark(t *testing.T) {
	dir := liveSession(t, "api")
	peer := otherRun(t, dir, "web")
	readFrom(t, dir, peer)
	self, _ := registry.Lookup(dir, "api")
	turnStarted(t, dir, self, markAt-1)
	if code, _, errOut := run("pending", "the suite is running"); code != ExitOK {
		t.Fatalf("pending: %s", errOut)
	}
	if err := completeTurn(dir, self, inbox.TurnEnd{Boundary: boundaryNow(t, dir, self), ID: "t/1", Text: " \n", Started: markAt - 1, Ended: boottime.Now()}, "t"); err != nil {
		t.Fatal(err)
	}
	got := reportsTo(t, dir, "web")
	if len(got) != 1 || inbox.KindOf(got[0]) != inbox.Interim || got[0].Text != "the suite is running" {
		t.Fatalf("web holds %+v, want the mark's line alone", got)
	}
}

// Without a mark, a turn end is the report, as it always was.
func TestAnUnmarkedTurnEndStillReports(t *testing.T) {
	dir := liveSession(t, "api")
	peer := otherRun(t, dir, "web")
	readFrom(t, dir, peer)
	self, _ := registry.Lookup(dir, "api")
	if err := completeTurn(dir, self, inbox.TurnEnd{Boundary: boundaryNow(t, dir, self), ID: "t/1", Text: "done", Started: 1, Ended: boottime.Now()}, "t"); err != nil {
		t.Fatal(err)
	}
	if got := kinds(reportsTo(t, dir, "web")); len(got) != 1 || got[0] != "finished" {
		t.Fatalf("web holds %v", got)
	}
}

// A failure or a stop says more than "still going": the mark does not soften
// either, and stays, since no end removes a mark.
func TestAFailedOrStoppedTurnIgnoresTheMark(t *testing.T) {
	for name, event := range map[string]inbox.TurnEnd{
		"error":   {ID: "t/1", Failed: true, Text: "the provider refused"},
		"stopped": {ID: "t/1", Stopped: true, Text: "the person stopped it"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := liveSession(t, "api")
			peer := otherRun(t, dir, "web")
			readFrom(t, dir, peer)
			self, _ := registry.Lookup(dir, "api")
			turnStarted(t, dir, self, markAt-1)
			if code, _, errOut := run("pending", "waiting"); code != ExitOK {
				t.Fatalf("pending: %s", errOut)
			}
			event.Started, event.Ended, event.Boundary = markAt-1, boottime.Now(), boundaryNow(t, dir, self)
			if err := completeTurn(dir, self, event, "t"); err != nil {
				t.Fatal(err)
			}
			if got := kinds(reportsTo(t, dir, "web")); len(got) != 1 || got[0] != name {
				t.Fatalf("web holds %v, want %s", got, name)
			}
			if _, ok, _ := markWithin(dir, "api", self.Epoch(), 1, boottime.Now()); !ok {
				t.Error("the turn end that ignored the mark removed it")
			}
		})
	}
}

// Refused outside a session, for main, with nothing owed, without text, and
// on Claude Code when no turn start has been recorded.
func TestPendingIsRefusedWhereItMeansNothing(t *testing.T) {
	dir := liveSession(t, "api")
	self, _ := registry.Lookup(dir, "api")
	t.Setenv(state.SessionEnv, "api")
	t.Setenv(epochEnv, self.Epoch())
	if code, _, errOut := run("pending", "x"); code != ExitUsage || !strings.Contains(errOut, "records no turn starts") {
		t.Errorf("no turn start: %d %s", code, errOut)
	}
	turnStarted(t, dir, self, markAt-1)
	if code, _, errOut := run("pending", "x"); code != ExitUsage || !strings.HasPrefix(errOut, "nothing is owed a report, so there is nothing to keep open; end the turn as usual.\n") {
		t.Errorf("nothing owed: %d %s", code, errOut)
	}
	if code, _, errOut := run("pending", " "); code != ExitUsage || !strings.Contains(errOut, "needs the text") {
		t.Errorf("empty: %d %s", code, errOut)
	}
	markMain(t, dir, "api")
	if code, _, errOut := run("pending", "x"); code != ExitUsage || !strings.Contains(errOut, "reported to nobody") {
		t.Errorf("main: %d %s", code, errOut)
	}
	t.Setenv(state.SessionEnv, "")
	if code, _, errOut := run("pending", "x"); code != ExitUsage || !strings.Contains(errOut, "this is not one") {
		t.Errorf("outside a session: %d %s", code, errOut)
	}
}

// A mark set in K whose end was lost; K+1's end is heard, and raises the
// recorded start to it; K+2 starts without a UserPromptSubmit and ends with
// its final answer, which is a report.
func TestAHeardTurnEndCorrectsALostOne(t *testing.T) {
	dir := liveSession(t, "api")
	peer := otherRun(t, dir, "web")
	readFrom(t, dir, peer)
	self, _ := registry.Lookup(dir, "api")
	turnStarted(t, dir, self, markAt-100)
	if err := markPending(dir, "api", self.Epoch(), "waiting in K", markAt-50); err != nil {
		t.Fatal(err)
	}
	// K's end is lost. K+1 ends and is heard: this process's start is its end.
	if code, _, errOut := run("turn-ended", `{"hook_event_name":"Stop","last_assistant_message":"K+1 said something"}`); code != ExitOK {
		t.Fatalf("turn-ended: %s", errOut)
	}
	if got := recordedStart(t, dir, self); got != markAt {
		t.Fatalf("the recorded start is %d after a heard end at %d", got, markAt)
	}
	// K+2 starts by itself, with no UserPromptSubmit, and ends with the answer.
	readFrom(t, dir, peer)
	if err := completeTurn(dir, self, inbox.TurnEnd{Boundary: boundaryNow(t, dir, self), ID: "t/K+2", Text: "the final answer", Started: recordedStart(t, dir, self), Ended: boottime.Now()}, "t"); err != nil {
		t.Fatal(err)
	}
	var final bool
	for _, message := range reportsTo(t, dir, "web") {
		final = final || inbox.KindOf(message) == inbox.Finished && message.Text == "the final answer"
	}
	if !final {
		t.Fatalf("K+2's answer did not leave as a report: %v", kinds(reportsTo(t, dir, "web")))
	}
}

// markPending makes a mark of its own, as one rewake pending call does.
func markPending(dir, name, epoch, text string, at int64) error {
	return inbox.MarkPending(dir, name, epoch, inbox.MarkName(at, inbox.NewID()), text, at)
}

// markWithin answers the line of the mark that decides a turn end whose
// window is (start, ended].
func markWithin(dir, name, epoch string, start, ended int64) (string, bool, error) {
	mark, ok, err := inbox.MarkWithin(dir, name, epoch, start, ended)
	return mark.Text, ok, err
}
