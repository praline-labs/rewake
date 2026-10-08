package workflow

import (
	"strings"
	"testing"
	"time"
)

// TestASecondTerminalProducesNoSecondReport is the other half of the question
// the failure controls left open.
//
// A turn ends once. If a server were to end it twice — a retry, a race, a
// confused reconnect — the second event must not turn into a second report to
// the sender, who would then be told twice about work done once.
//
// This is stated as its own case rather than as an observation of the delivery
// scenario, because it needs a peer that misbehaves: an honest shim never
// sends the second event, and an observation about something that never
// happens proves itself.
//
// It has no negative control inside the suite, and its negation turned out to
// be unreachable from here. Two of the three barriers were removed — the
// `w.done` guard in the observer (internal/harness/codex/gateway/observe.go)
// and the write that skips a report whose id is already in the mailbox
// (inbox.PutOnce, paired with the deterministic inbox.ReportID) — one at a
// time and together, and a single report still came back.
//
// What seems to hold it is the third: the obligations cleared once a report is
// published (internal/cli/turn_reports.go). After the first report the waiter
// has no messages left, so a second outcome has nothing to report about. That
// is a reading of the code, not a result — it is the one barrier no
// single-line mutation could remove, so it stayed untested.
//
// So this case states that the system as a whole answers once, and proves
// nothing about any single barrier. A green run here is worth less than a
// green run of a case with a working control, and saying which is the point —
// see the boundary in check-runner-scenarios.md.
func TestASecondTerminalProducesNoSecondReport(t *testing.T) {
	binary := enterScenario(t, "second-terminal")

	c := Start(t, Spec{
		Name:    "second-terminal",
		Harness: "codex",
		Observations: []string{
			"the recipient ends the same turn twice",
			"the sender is told once",
		},
		Deadline: 90 * time.Second,
	})
	iso := Isolate(t, c, binary)

	worker := startCodexSession(t, c, iso, "worker", "--general",
		shimInboxJSON+"=1", shimSecondTerminal+"=1")
	defer stopSession(t, c, worker)
	sender := startCodexSession(t, c, iso, "sender", "--main",
		shimSendTo+"="+worker.name, shimSendText+"="+taskText, shimInboxJSON+"=1")
	defer stopSession(t, c, sender)

	// The recipient's own record, not an inference from the sender's mailbox:
	// "the second report never arrived" is also what a scenario would see if
	// the second terminal had never been sent.
	c.Await("the recipient to end its turn twice", func() bool {
		return strings.Contains(worker.acceptedTurns(), "terminal-again")
	})
	c.Observed("the recipient ends the same turn twice", "the fixture sent a second terminal event")

	c.Await("the sender to read the report", func() bool {
		_, found := reportOfKind(sender, worker, "finished")
		return found
	})
	// One report has arrived; a second one, if it is coming, comes behind it.
	// Judging immediately would pass on the gap rather than on the rule.
	time.Sleep(reportWindow)

	if reports := countReports(sender, worker, "finished"); reports != 1 {
		c.Contradicted("the sender is told once",
			"%d finished reports for one turn", reports)
		return
	}
	c.Observed("the sender is told once", "one report for a turn that ended twice")
}
