package workflow

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestTaskReport is the delivery scenario: a task reaches an idle session, that
// session consumes it, and the report that comes back corresponds to the
// message that was consumed.
//
// The weak version of that claim — "something was sent and something came
// back" — proves itself and costs nothing. Everything here is about the two
// "exactly": exactly that session, exactly that message.
//
// Two sessions, because the second half of the invariant lives on the sender's
// side: a message sent by the test itself has no session for a report to
// return to, and half the claim would go unchecked. Both are equally honest
// shims — the sender sends with its own rewake and reads its own mailbox.
//
// What this does not prove: that a model read the message and decided to
// answer. This is the fixture tier; the only observation of that kind is the
// live one of September 21, 2026 in claude-parity-2026-09-21.md. A green run
// here must never be read as evidence of understanding.
func TestTaskReport(t *testing.T) {
	runInColumns(t, "task-report", runTaskReport)
}

func runTaskReport(t *testing.T, col column) {
	binary := enterScenario(t, "task-report")

	c := Start(t, Spec{
		Name:    "task-report",
		Harness: col.harness,
		Observations: []string{
			"the recipient accepts the delivered turn",
			"the recipient read its own mailbox",
			"the report reaches the sender",
			"the report corresponds to the message that was delivered",
			"both sessions are still running when the case is judged",
		},
		Deadline: 90 * time.Second,
	})
	iso := Isolate(t, c, binary)

	// The recipient must not be main: a main is silent by design — reading its
	// mail records no obligation to answer, and its finished turn produces no
	// message. Asking it for a report would be asking the one role forbidden
	// to give one. Main goes to the sender, which is also the side whose
	// telemetry the scenario needs to watch.
	// Both sessions read their mail in the machine form: the recipient so the
	// scenario can tell which message it consumed, the sender so it can read
	// the link a report carries. Neither is visible in the printed form.
	worker := startHarnessSession(t, c, iso, col.harness, "worker", "--general", shimInboxJSON+"=1")
	defer stopSession(t, c, worker)
	sender := startHarnessSession(t, c, iso, col.harness, "sender", "--main",
		shimSendTo+"="+worker.name, shimSendText+"="+taskText, shimInboxJSON+"=1",
		readinessSwitch(worker))
	defer stopSession(t, c, sender)

	// The recipient proves it can receive by the file it writes, which the
	// sender waits for before the task leaves (readinessSwitch).

	// The recipient's own read is the proof that the turn was accepted and
	// that a session, not the test, consumed the mail: the shim refuses a turn
	// whose notice is not a mailbox delivery with identities and epochs, so
	// reaching a read at all means the turn passed those checks.
	read := ""
	c.Await("the recipient to read the task", func() bool {
		read = worker.mailboxRead()
		return strings.Contains(read, taskText)
	})
	c.Observed("the recipient accepts the delivered turn", "the delivery passed the shim's checks on the notice")
	c.Observed("the recipient read its own mailbox", "the session's own inbox call returned the task")

	// And the report: read by the sender, with its own rewake.
	var report reportView
	found := false
	c.Await("the sender to read the report", func() bool {
		report, found = reportOfKind(sender, worker, "finished")
		return found
	})
	if !found {
		c.Contradicted("the report reaches the sender", "no finished report from %s", worker.name)
		return
	}
	c.Observed("the report reaches the sender", "read by the sender's own inbox call")

	// The correlation, and the whole point of the scenario: the report names
	// the messages it settles, and the delivery named what arrived. Comparing
	// the two is the only check that survives a report carrying the right text
	// about the wrong message — the text, after all, is whatever the recipient
	// chose to say.
	task, known := messageCarrying(worker, taskText)
	if !known {
		c.Contradicted("the report corresponds to the message that was delivered",
			"the recipient's own read does not say which message carried the task")
		return
	}
	if !col.deliveryNamed(worker, task.ID) {
		c.Contradicted("the report corresponds to the message that was delivered",
			"the task read as %s was not among the delivered %v", task.ID, worker.deliveredIDs())
		return
	}
	// Equality, not overlap. A report that settles the task *and something
	// else* is answering about a message this scenario never sent, and an
	// overlap test would call that correct — the acceptance round of
	// September 21, 2026 showed exactly that by adding a foreign id beside the
	// real one and watching the scenario stay green.
	if !slices.Equal(report.InReplyTo, []string{task.ID}) {
		c.Contradicted("the report corresponds to the message that was delivered",
			"the report settles %v; the task consumed here was %s", report.InReplyTo, task.ID)
		return
	}
	c.Observed("the report corresponds to the message that was delivered",
		"settles exactly "+task.ID+", the message the recipient consumed")

	// Last, and deliberately after everything else: every observation above
	// was read from a file, and a file outlives the session that wrote it. A
	// session that had already left would make all of them describe something
	// that is no longer there.
	if !worker.alive() || !sender.alive() {
		c.Contradicted("both sessions are still running when the case is judged",
			"worker alive: %v, sender alive: %v", worker.alive(), sender.alive())
		return
	}
	c.Observed("both sessions are still running when the case is judged",
		"neither session left before the verdict")
}

// countReports is how many messages of one kind a session read from another.
func countReports(reader, about *scenarioSession, kind string) int {
	seen := 0
	for _, message := range readMessages(reader) {
		if message.From == about.name && message.Kind == kind {
			seen++
		}
	}
	return seen
}

// reportView is the part of a message a scenario about reports needs: who sent
// it, what kind it is, and which messages it settles.
type reportView struct {
	ID        string   `json:"id"`
	From      string   `json:"from"`
	Kind      string   `json:"kind"`
	InReplyTo []string `json:"inReplyTo"`
	Text      string   `json:"text"`
	// ThreadChanged is the reader's warning that it answered from another
	// conversation than the one the message was delivered to.
	ThreadChanged bool `json:"threadChanged"`
	// Undelivered is set on the note that tells a sender its message never
	// reached the agent.
	Undelivered *struct {
		ID   string `json:"id"`
		Kind string `json:"kind"`
	} `json:"undelivered"`
}

// messageCarrying finds, among what a session's own inbox calls returned, the
// message whose text carries a marker. That is how the scenario learns the id
// of the task the recipient actually consumed, rather than taking the word of
// the report it is about to check.
func messageCarrying(reader *scenarioSession, marker string) (reportView, bool) {
	for _, message := range readMessages(reader) {
		if strings.Contains(message.Text, marker) {
			return message, true
		}
	}
	return reportView{}, false
}

// reportOfKind looks through what a session's own inbox calls returned for a
// message of one kind from another session. Availability notices also come
// "from" that session, and are not answers to anything, so the kind is part of
// the question.
func reportOfKind(reader, about *scenarioSession, kind string) (reportView, bool) {
	for _, message := range readMessages(reader) {
		if message.From == about.name && message.Kind == kind {
			return message, true
		}
	}
	return reportView{}, false
}

// readMessages are the messages a session's own inbox calls returned, in the
// order they were read. The file holds one machine-form read per record.
func readMessages(reader *scenarioSession) []reportView {
	var all []reportView
	for _, chunk := range strings.Split(reader.mailboxRead(), "\n---\n") {
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		var model struct {
			Messages []reportView `json:"messages"`
		}
		if json.Unmarshal([]byte(chunk), &model) != nil {
			continue
		}
		all = append(all, model.Messages...)
	}
	return all
}

// taskText is distinctive so that finding it in a report means it came from
// this message and not from anything else in the mailbox.
const taskText = "task-report-probe-please-answer"

// stopSession ends a session and reports a failure to end as a test failure.
func stopSession(t *testing.T, c *Case, session *scenarioSession) {
	t.Helper()
	if err := session.stop(c); err != nil {
		t.Errorf("ending %s: %v", session.name, err)
	}
}
