package workflow

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	// turnWindow is how long a delivery may take to reach the recipient, and
	// how long a report a world is declared to send may take to arrive. It
	// bounds a wait that a healthy run finishes in milliseconds; reaching it
	// is a finding, not a pass.
	turnWindow = 30 * time.Second
	// reportWindow is how long a world declared to send nothing is watched
	// before its silence is believed. Absence needs a window of some length;
	// this one is several times what a report takes when there is one.
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

// runTaskReportControl runs the delivery scenario's sessions in one control's
// world and records whether the named observation came out the way this run
// requires. The case starts before anything is built, so a mutant that fails
// to build is a named red case with its build directory kept.
func runTaskReportControl(t *testing.T, col column, name string, control taskReportControl, expected string, want whatIsExpected) {
	t.Helper()
	c := Start(t, Spec{
		Name:         name,
		Harness:      col.harness,
		Observations: []string{want.observation()},
		Deadline:     90 * time.Second,
	})
	binary := suite.binary
	if control.mutant != nil {
		built, err := buildMutant(c, *control.mutant)
		if err != nil {
			c.Contradicted(want.observation(), "the mutant could not be built: %v", err)
			return
		}
		binary = built
	}
	iso := Isolate(t, c, binary)
	worker := startHarnessSession(t, c, iso, col.harness, "worker", "--general",
		append([]string{shimInboxJSON + "=1"}, control.shim...)...)
	defer stopSession(t, c, worker)
	sender := startHarnessSession(t, c, iso, col.harness, "sender", "--main",
		shimSendTo+"="+worker.name, shimSendText+"="+taskText, shimInboxJSON+"=1",
		readinessSwitch(col, worker))
	defer stopSession(t, c, sender)

	// Anchored on the recipient's own record of a turn: until one has arrived
	// there is nothing to judge, and asking early would let a control pass on
	// the report not having come back *yet*.
	if !waitFor(c, turnWindow, func() bool { return worker.acceptedTurns() != "" }) {
		c.Contradicted(want.observation(), "no turn ever reached the recipient, so nothing could be judged")
		return
	}
	// Then what this world is declared to send: waited for when it sends
	// something, and watched for a while when it sends nothing.
	window := turnWindow
	if control.reports == reportsNothing {
		window = reportWindow
	}
	waitFor(c, window, func() bool { return outcomesRead(sender, worker) > 0 })
	if control.leaves {
		// Anchored on the departure itself, bounded: a world declared to
		// leave that never does leaves the observation standing, and the
		// control goes red for it.
		waitFor(c, turnWindow, func() bool { return !worker.alive() })
	}
	c.Note("watching " + expected + " under " + control.name)

	if sent := sentKind(sender, worker); sent != control.reports {
		// The crosswise check decides which cells to ask from this
		// declaration, so a world that sends something else is a defect in
		// the table, not a detail of this run.
		c.Contradicted(want.observation(), "the %s world is declared to send %s and sent %s", control.name, control.reports, sent)
		return
	}
	broke, why, err := taskReportOutcome(col, expected, worker, sender)
	switch {
	case err != nil:
		c.Contradicted(want.observation(), "could not judge %s under %s: %v", expected, control.name, err)
	case want == mustBreak && broke:
		c.Observed(want.observation(), why)
	case want == mustBreak:
		c.Contradicted(want.observation(), "%s held anyway: %s", expected, why)
	case broke:
		c.Contradicted(want.observation(), "%s broke under %s, whose breakage it does not cover: %s", expected, control.name, why)
	default:
		c.Observed(want.observation(), fmt.Sprintf("%s held under %s: %s", expected, control.name, why))
	}
}

// sentKind is what the sender actually read back from the recipient.
func sentKind(sender, worker *codexSession) reportKind {
	if _, finished := reportOfKind(sender, worker, "finished"); finished {
		return reportsFinished
	}
	if _, failed := reportOfKind(sender, worker, "error"); failed {
		return reportsError
	}
	return reportsNothing
}

// taskReportOutcome asks for the whole shape of each breakage, in three
// values: broken, not broken, or not judgeable. Each branch requires what its
// question stands on and refuses to answer without it.
func taskReportOutcome(col column, expected string, worker, sender *codexSession) (bool, string, error) {
	read := strings.Contains(worker.mailboxRead(), taskText)
	sent := sentKind(sender, worker)
	switch expected {
	case obsTRRead:
		if !read && sent == reportsNothing {
			return true, "a turn was delivered, nothing was read, and no report followed", nil
		}
		return false, "the recipient read the task", nil
	case obsTRReaches:
		if !read {
			return false, "", errors.New("the task was never read, so no report was owed")
		}
		if sent == reportsNothing {
			return true, "the task was read and no report of any kind reached the sender", nil
		}
		return false, "a " + string(sent) + " report reached the sender", nil
	case obsTRFinished:
		if sent == reportsNothing {
			return false, "", errors.New("no report arrived, so its kind cannot be judged")
		}
		if sent == reportsError {
			return true, "the turn reported an error rather than an answer", nil
		}
		return false, "the report is a finished answer", nil
	case obsTRCorrelates:
		report, finished := reportOfKind(sender, worker, "finished")
		task, known := messageCarrying(worker, taskText)
		if !finished || !known {
			return false, "", errors.New("no finished report, or no consumed task, to correlate")
		}
		// Either half broken is the breakage: the delivery naming something
		// other than what was consumed, or the report settling something
		// other than exactly that.
		if !col.deliveryNamed(worker, task.ID) || !slices.Equal(report.InReplyTo, []string{task.ID}) {
			return true, fmt.Sprintf("the task consumed was %s, the delivery named %v, the report settles %v",
				task.ID, worker.deliveredIDs(), report.InReplyTo), nil
		}
		return false, "the report settles exactly the message the delivery named", nil
	case obsTRAlive:
		if !worker.alive() || !sender.alive() {
			return true, fmt.Sprintf("a session left before the verdict: worker %v, sender %v", worker.alive(), sender.alive()), nil
		}
		return false, "both sessions are running", nil
	}
	return false, "", errors.New("no such observation: " + expected)
}
