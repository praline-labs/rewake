package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestUnreadReminder is the reminder of a task a finished turn left unread
// (docs/delivery.md#the-notice), on the fixture column: the case seen live was
// a worker that ended a turn with a task announced in it and never read, and
// nothing woke it again.
//
//   - main sends a task; the worker's turn reads it and stays open, and main's
//     second task is steered into that turn, which ends without reading it.
//     One reminder follows, naming that task alone.
//   - The reminded turn reads nothing either and ends: no second reminder.
//   - A note wakes the worker once it reads again; it reads the task, and the
//     task's report reaches main.
//
// What it does not prove: that a real harness takes a reminder right after a
// turn's end as it takes any notice; the reminder goes through the same Wake.
func TestUnreadReminder(t *testing.T) {
	binary := enterScenario(t, "unread-reminder")
	c := Start(t, Spec{
		Name:         "unread-reminder",
		Harness:      fixtureColumn.harness,
		Observations: reminderObservations,
		Deadline:     90 * time.Second,
	})
	for _, finding := range playReminder(t, c, Isolate(t, c, binary)) {
		if finding.held {
			c.Observed(finding.observation, finding.detail)
		} else {
			c.Contradicted(finding.observation, "%s", finding.detail)
		}
	}
}

const (
	obsReminderOnce     = "a task steered into a turn that ends without reading it brings one reminder, naming that task"
	obsReminderNotAgain = "the reminded turn, ending with the task still unread, brings no second reminder"
	obsReminderReport   = "the task, read at last, has its report reach its sender"
	reminderFirst       = "unread-reminder: the first task"
	reminderSecond      = "unread-reminder: the task left unread"
	reminderNote        = "unread-reminder: read your mail"
	reminderLine        = "still waiting"
)

var reminderObservations = []string{obsReminderOnce, obsReminderNotAgain, obsReminderReport}

func playReminder(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	t.Helper()
	col := fixtureColumn.harness
	leave := filepath.Join(iso.Home, "worker.leave-unread")
	worker := startHarnessSession(t, c, iso, col, "worker", "--general", shimInboxJSON+"=1", shimHoldTurn+"=1", shimLeaveUnread+"="+leave)
	defer stopSession(t, c, worker)
	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, col, "lead", "--main", shimInboxJSON+"=1", asks.env())
	defer stopSession(t, c, lead)

	finding := func(observation string, held bool, detail string, args ...any) telemetryFinding {
		return telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)}
	}
	unjudged := func(detail string, args ...any) []telemetryFinding {
		var out []telemetryFinding
		for _, observation := range reminderObservations {
			out = append(out, telemetryFinding{observation: observation, detail: fmt.Sprintf(detail, args...)})
		}
		return out
	}
	reminders := func() []groupDelivery {
		deliveries, err := worker.groupDeliveries()
		if err != nil {
			c.t.Fatalf("workflow: %v", err)
		}
		var out []groupDelivery
		for _, delivery := range deliveries {
			if strings.Contains(delivery.Notice, reminderLine) {
				out = append(out, delivery)
			}
		}
		return out
	}
	completions := func() int {
		events, err := worker.turnEvents()
		if err != nil {
			c.t.Fatalf("workflow: %v", err)
		}
		return len(allOf(events, "completed"))
	}

	if err := sessionsUp(c, worker, lead); err != nil {
		return unjudged("%v", err)
	}
	if code, out, _ := asks.ask(c, "send", worker.name, reminderFirst); code != 0 {
		return unjudged("the first task was not sent: exit %d, %s", code, firstLine(out))
	}
	if !waitFor(c, 20*time.Second, func() bool {
		events, err := worker.turnEvents()
		return err == nil && eventOf(events, "operation-open") != nil
	}) {
		return unjudged("the worker never held its first turn open")
	}
	// From here the worker reads nothing: the task steered in stays unread.
	if err := os.WriteFile(leave, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, _ := asks.ask(c, "send", worker.name, reminderSecond)
	second := sentID(out)
	if code != 0 || second == "" {
		return unjudged("the second task was not sent: exit %d, %s", code, firstLine(out))
	}

	var found []telemetryFinding
	waitFor(c, 20*time.Second, func() bool { return len(reminders()) > 0 && completions() >= 2 })
	first := reminders()
	switch {
	case len(first) == 0:
		deliveries, _ := worker.groupDeliveries()
		found = append(found, finding(obsReminderOnce, false, "no reminder came; the worker's deliveries: %+v", deliveries))
	default:
		found = append(found, finding(obsReminderOnce, slices.Equal(first[0].Members, []string{second}),
			"the reminder named %v in %q, the task left unread is %s", first[0].Members, first[0].Notice, second))
	}
	// A second reminder would follow the reminded turn's end at once; the
	// look waits a little past it.
	time.Sleep(2 * time.Second)
	after := reminders()
	found = append(found, finding(obsReminderNotAgain, len(after) == 1 && completions() >= 2,
		"%d reminders after %d turns ended", len(after), completions()))

	if err := os.Remove(leave); err != nil {
		t.Fatal(err)
	}
	if code, out, _ := asks.ask(c, "send", worker.name, reminderNote, "--notify"); code != 0 {
		found = append(found, finding(obsReminderReport, false, "the note was not sent: exit %d, %s", code, firstLine(out)))
		return found
	}
	answered := func() bool {
		for _, message := range readMessages(lead) {
			if message.From == worker.name && message.Kind == "finished" && slices.Contains(message.InReplyTo, second) {
				return true
			}
		}
		return false
	}
	waitFor(c, 30*time.Second, answered)
	var read []reportView
	for _, message := range readMessages(lead) {
		if message.From == worker.name {
			read = append(read, message)
		}
	}
	found = append(found, finding(obsReminderReport, answered(), "main read from %s: %v", worker.name, reportKinds(read)))
	return found
}

// sentID is the id a send printed on a line of its own.
func sentID(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if id, ok := strings.CutPrefix(strings.TrimSpace(line), "id "); ok {
			return id
		}
	}
	return ""
}
