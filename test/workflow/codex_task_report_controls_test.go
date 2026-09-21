package workflow

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// The scenario's negative controls. Each one breaks exactly one thing and must
// take down exactly the observation that covers it — otherwise the scenario
// would pass on a shim and a wrapper agreeing with each other.
//
// Every one of them is verified twice over: with its own breakage removed it
// must go red, and with another control's breakage in place it must go red
// too. The first version of these controls passed both ways, which is the
// failure this suite exists to catch.

// A report whose link points at a message that never arrived says nothing
// about which task it answers. The correlation observation must notice.
func TestReportNotMatchingTheMessageFails(t *testing.T) {
	failedTaskReport(t, "wrong-report", "the report corresponds to the message that was delivered",
		shimWrongReportID+"=1")
}

// A delivery alone is not the claim: if the session could not read its mail,
// the case must not go green on the fact that something was delivered.
func TestAFailedMailboxReadFails(t *testing.T) {
	failedTaskReport(t, "read-fails", "the recipient read its own mailbox",
		shimReadFails+"=1")
}

// A session that sends its answer and then fails has not answered. The failure
// here falls between the two: after the text of the turn, before the report is
// published. The report must arrive as an error, not as the finished answer
// whose text it carries.
//
// The name is exact on purpose. A failure arriving *after* the report was
// published is a different question, and this control does not ask it — see
// the boundary in check-runner-scenarios.md for what can and cannot be asked
// about one at this tier.
func TestAFailureBeforeTheReportIsPublished(t *testing.T) {
	failedTaskReport(t, "failure-before-report", "the report is a finished answer",
		shimLateFailure+"=1")
}

// failedTaskReport runs the delivery scenario with one thing broken and
// requires that the named observation is the one that fails.
func failedTaskReport(t *testing.T, name, expected string, controls ...string) {
	t.Helper()
	binary := enterScenario(t, "task-report-control-"+name)

	c := Start(t, Spec{
		Name:         "task-report-control-" + name,
		Harness:      "codex",
		Observations: []string{"the control broke what it meant to break"},
		Deadline:     90 * time.Second,
	})
	iso := Isolate(t, c, binary)

	worker := startCodexSession(t, c, iso, "worker", "--general",
		append([]string{shimInboxJSON + "=1"}, controls...)...)
	defer stopSession(t, c, worker)
	sender := startCodexSession(t, c, iso, "sender", "--main",
		shimSendTo+"="+worker.name, shimSendText+"="+taskText, shimInboxJSON+"=1")
	defer stopSession(t, c, sender)

	// Anchored on the recipient's own record of the turn rather than on a flat
	// sleep: until a turn has arrived there is nothing to judge, and asking
	// early would let a control pass on the mere fact that the report has not
	// come back *yet* — which is true of a healthy run too, for a moment.
	if !waitFor(c, turnWindow, func() bool { return strings.Contains(worker.acceptedTurns(), "turn-") }) {
		c.Contradicted("the control broke what it meant to break", "no turn ever reached the recipient")
		return
	}
	c.Note("watching for " + expected + " to fail")
	time.Sleep(reportWindow)

	if broke, why := controlOutcome(expected, worker, sender); broke {
		c.Observed("the control broke what it meant to break", why)
		return
	}
	c.Contradicted("the control broke what it meant to break",
		"%s held anyway: worker turns %q, delivered %v", expected,
		firstLine(worker.acceptedTurns()), worker.deliveredIDs())
}

const (
	// turnWindow is how long a delivery may take to reach the recipient. It
	// bounds a wait that a healthy run finishes in milliseconds; a control
	// that never gets its turn is a broken case, not a passing one.
	turnWindow = 30 * time.Second
	// reportWindow is what a report needs once the turn has been recorded: the
	// read, the terminal event, and the sender's own next inbox call. The
	// healthy scenario does all three inside a fraction of this.
	reportWindow = 1500 * time.Millisecond
)

// waitFor polls until the condition holds or the window runs out.
func waitFor(c *Case, window time.Duration, condition func() bool) bool {
	deadline := time.Now().Add(window)
	for time.Now().Before(deadline) && !c.Expired() {
		if condition() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// controlOutcome reports whether the expected observation has been broken.
//
// Each branch asks for the whole shape of its breakage, not just its symptom:
// the cross-check showed that a branch naming only what is missing goes green
// on another control's breakage as well.
func controlOutcome(expected string, worker, sender *codexSession) (bool, string) {
	read, delivered := worker.mailboxRead(), worker.deliveredIDs()
	report, reported := reportOfKind(sender, worker, "finished")
	switch expected {
	case "the recipient read its own mailbox":
		// A turn was delivered and nothing was read, so no obligation to answer
		// was ever recorded and no report can exist. Delivery alone is what is
		// left — which is exactly what must not be enough.
		if !strings.Contains(read, taskText) && !reported {
			return true, "a turn was delivered, nothing was read, and no report followed"
		}
	case "the report corresponds to the message that was delivered":
		// The report came back, and what it settles is not what arrived here.
		// An observation that only asked whether a report exists would call
		// this a pass.
		task, known := messageCarrying(worker, taskText)
		if reported && known && !slices.Contains(delivered, task.ID) &&
			!slices.Equal(report.InReplyTo, delivered) {
			return true, fmt.Sprintf("the delivery named %v, the task consumed was %s, and the report settles %v",
				delivered, task.ID, report.InReplyTo)
		}
	case "the report is a finished answer":
		// The read happened and the text was sent; the kind is what says the
		// turn did not answer. Requiring the read too is what keeps this branch
		// from passing on a broken read instead.
		if _, failed := reportOfKind(sender, worker, "error"); strings.Contains(read, taskText) && !reported && failed {
			return true, "the failed turn reported an error rather than an answer"
		}
	}
	return false, ""
}
