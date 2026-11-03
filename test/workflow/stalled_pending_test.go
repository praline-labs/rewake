package workflow

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestStalledPending is a stalled pending end to end: a worker reads main's
// task, ends its turn with rewake pending and is left idle with nothing to
// wake it. main must see it so — rewake list and rewake inbox --awaited say
// pending, idle — and past the bound the worker's own wrapper must wake it
// with one notify of its own quoting the pending line; the turn that starts
// ends unmarked, so its end is the report that settles the task.
//
// The binary is the suite's with the bound cut to stallBound (internal/host/
// delivery, builtStall): at its real ten minutes the case would wait out all
// of it, and the suite's own binary keeps the release's, so no other case
// that leaves a worker pending is woken by it.
func TestStalledPending(t *testing.T) {
	enterScenario(t, "stalled-pending")
	stalled, err := buildRewakeWith(t.TempDir(), buildValue("host/delivery", "builtStall", stallBound))
	if err != nil {
		t.Fatal(err)
	}
	c := Start(t, Spec{Name: "stalled-pending", Harness: fixtureColumn.harness, Observations: stallObservations, Deadline: 120 * time.Second})
	for _, finding := range playStalledPending(t, c, Isolate(t, c, stalled)) {
		if finding.held {
			c.Observed(finding.observation, finding.detail)
		} else {
			c.Contradicted(finding.observation, "%s", finding.detail)
		}
	}
}

// stallBound is long enough for main to read the interim message — which
// waits out the coalescing quiet — and ask its views before the worker is
// woken, on a loaded machine.
const stallBound = 12 * time.Second

const (
	obsStallShown    = "while the worker is idle under its pending mark, main's rewake list shows it pending, idle, and rewake inbox --awaited marks its task so"
	obsStallWoken    = "past the bound the worker reads one notify of its own quoting its pending line and naming main"
	obsStallReported = "the turn the reminder started ends with the report that settles the task"
	stallTask        = "stalled-pending: run the long check"
	stallPending     = "waiting for another session's run to end"
)

var stallObservations = []string{obsStallShown, obsStallWoken, obsStallReported}

func playStalledPending(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	t.Helper()
	harness := fixtureColumn.harness
	worker := startHarnessSession(t, c, iso, harness, "worker", "--general", shimInboxJSON+"=1", shimPendingOnce+"="+stallPending)
	defer stopSession(t, c, worker)
	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, harness, "lead", "--main", shimInboxJSON+"=1", asks.env())
	defer stopSession(t, c, lead)

	finding := func(observation string, held bool, detail string, args ...any) telemetryFinding {
		return telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)}
	}
	unjudged := func(observations []string, detail string) []telemetryFinding {
		var out []telemetryFinding
		for _, observation := range observations {
			out = append(out, telemetryFinding{observation: observation, detail: detail})
		}
		return out
	}
	readAbout := func(task, kind string) bool {
		for _, message := range readMessages(lead) {
			if message.From == worker.name && message.Kind == kind && slices.Contains(message.InReplyTo, task) {
				return true
			}
		}
		return false
	}
	reminders := func() []reportView {
		var found []reportView
		for _, message := range readMessages(worker) {
			if message.From == worker.name && message.Kind == "notify" && strings.HasPrefix(message.Text, "Rewake: your pending ") {
				found = append(found, message)
			}
		}
		return found
	}

	var sent string
	if err := sessionsUp(c, worker); err != nil {
		sent = err.Error()
	} else {
		code, out, _ := asks.ask(c, "send", worker.name, stallTask)
		sent = fmt.Sprintf("exit %d, %s", code, firstLine(out))
	}
	var task reportView
	if !waitFor(c, 30*time.Second, func() bool {
		var ok bool
		task, ok = messageCarrying(worker, stallTask)
		return ok && readAbout(task.ID, "pending")
	}) {
		return unjudged(stallObservations, fmt.Sprintf("main never read an interim message about the task (send: %s); kinds main read: %v; the worker's turns: %s",
			sent, kindsFrom(lead, worker), worker.acceptedTurns()))
	}

	var out []telemetryFinding
	_, listed, _ := asks.ask(c, "list")
	_, awaited, _ := asks.ask(c, "inbox", "--awaited")
	early := len(reminders()) == 0
	out = append(out, finding(obsStallShown, early && strings.Contains(lineWith(listed, worker.name), "pending, idle") &&
		strings.Contains(awaited, " · pending, idle <1m: "+stallPending),
		"asked before the reminder: %v; rewake list printed %q; --awaited printed %q", early, listed, awaited))

	var woken []reportView
	waitFor(c, stallBound+30*time.Second, func() bool {
		woken = reminders()
		return len(woken) > 0
	})
	quoted := len(woken) == 1 && strings.Contains(woken[0].Text, `"`+stallPending+`"`) && strings.Contains(woken[0].Text, "task of "+lead.name)
	out = append(out, finding(obsStallWoken, quoted, "reminders the worker read: %+v", woken))
	if len(woken) == 0 {
		return append(out, unjudged([]string{obsStallReported}, "no reminder woke the worker; its turns: "+worker.acceptedTurns())...)
	}

	reported := waitFor(c, 30*time.Second, func() bool { return readAbout(task.ID, "finished") })
	settled := reported && waitFor(c, 5*time.Second, func() bool { return len(owedTo(iso, worker, lead)) == 0 })
	return append(out, finding(obsStallReported, reported && settled && len(reminders()) == 1,
		"a finished report: %v; the task settled: %v; reminders read: %d; kinds main read: %v", reported, settled, len(reminders()), kindsFrom(lead, worker)))
}

// owedTo lists the worker's awaiting records of the sender's tasks.
func owedTo(iso *Isolation, worker, sender *scenarioSession) []string {
	found, _ := filepath.Glob(filepath.Join(iso.Room(iso.HomeRoom()), "inbox", worker.name, "awaiting", "*", sender.name+"*"))
	return found
}
