package workflow

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestPendingReport is `rewake pending` end to end. A worker reads a task and,
// in that first turn, runs `rewake pending` before the turn ends — a worker
// leaving background work running. The sender must be told the work is still
// going, in a message that is not a report and settles nothing — the mark's
// line first, then what the turn itself said — and the task must stay owed. Then the worker is woken by another session's task, its next
// turn ends with no mark, and that turn end is the report that settles the
// first task.
func TestPendingReport(t *testing.T) {
	runInColumns(t, "pending-report", func(t *testing.T, col column) {
		binary := enterScenario(t, "pending-report")
		c := Start(t, Spec{
			Name:         "pending-report",
			Harness:      col.harness,
			Observations: pendingObservations,
			Deadline:     90 * time.Second,
		})
		for _, finding := range playPendingReport(t, c, Isolate(t, c, binary), col) {
			if finding.held {
				c.Observed(finding.observation, finding.detail)
			} else {
				c.Contradicted(finding.observation, "%s", finding.detail)
			}
		}
	})
}

const (
	obsPendingMarked   = "the worker's rewake pending is accepted"
	obsInterimArrives  = "the sender reads an interim message about the task, the mark's line and then the turn's text"
	obsTaskStillOwed   = "the task stays owed after the interim turn end"
	obsReportFollows   = "the next turn end reports and settles the task"
	pendingText        = "the suite is running in the background"
	pendingTaskText    = "run the suite and report"
	pendingWakeText    = "the background work finished; wrap up"
	pendingSenderLabel = "sender"
)

var pendingObservations = []string{obsPendingMarked, obsInterimArrives, obsTaskStillOwed, obsReportFollows}

func playPendingReport(t *testing.T, c *Case, iso *Isolation, col column) []telemetryFinding {
	t.Helper()
	worker := startHarnessSession(t, c, iso, col.harness, "worker", "--general", shimInboxJSON+"=1", shimPendingOnce+"="+pendingText)
	defer stopSession(t, c, worker)
	sender := startHarnessSession(t, c, iso, col.harness, pendingSenderLabel, "--main",
		shimSendTo+"="+worker.name, shimSendText+"="+pendingTaskText, shimInboxJSON+"=1", readinessSwitch(col, worker))
	defer stopSession(t, c, sender)

	unjudged := func(detail string) []telemetryFinding {
		var out []telemetryFinding
		for _, observation := range pendingObservations {
			out = append(out, telemetryFinding{observation: observation, detail: detail})
		}
		return out
	}
	finding := func(observation string, held bool, detail string, args ...any) telemetryFinding {
		return telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)}
	}
	var out []telemetryFinding

	var task reportView
	if !waitFor(c, 30*time.Second, func() bool {
		var ok bool
		task, ok = messageCarrying(worker, pendingTaskText)
		return ok && strings.Contains(worker.acceptedTurns(), "pending ")
	}) {
		return unjudged("the worker never read the task and ran rewake pending: " + worker.acceptedTurns())
	}
	marked := strings.Contains(worker.acceptedTurns(), "pending ok")
	out = append(out, finding(obsPendingMarked, marked, "%s", firstLine(lineWith(worker.acceptedTurns(), "pending "))))

	// Anchored on the first thing the sender reads from the worker, whatever
	// it is: under a product that ignores the mark it is the report itself.
	var first reportView
	if !waitFor(c, 30*time.Second, func() bool {
		for _, message := range readMessages(sender) {
			if message.From == worker.name && slices.Contains(message.InReplyTo, task.ID) {
				first = message
				return true
			}
		}
		return false
	}) {
		return append(out, unjudged("the sender never read anything about the task from the worker")[1:]...)
	}
	// The turn's text is what the worker read: the task, among the rest.
	out = append(out, finding(obsInterimArrives, first.Kind == "pending" && strings.HasPrefix(first.Text, pendingText+"\n\n") && strings.Contains(first.Text, pendingTaskText),
		"the first message about the task was %s: %q", first.Kind, first.Text))

	owed := awaiting(iso, worker)
	out = append(out, finding(obsTaskStillOwed, len(owed) > 0, "awaiting records after the interim turn end: %v", owed))
	if len(owed) == 0 {
		// Settled already: no report can follow, and waiting for one would
		// only spend the case's time.
		return append(out, finding(obsReportFollows, false, "the task was settled before any report; kinds read by the sender: %v", kindsFrom(sender, worker)))
	}

	// The wake: another session's task starts the worker's next turn, which
	// ends without a mark. A session rather than a shell, because the Codex
	// path refuses mail from outside a session.
	// No readiness wait: the worker has taken mail already, and a general
	// session cannot see the telemetry that wait reads.
	waker := startHarnessSession(t, c, iso, col.harness, "waker", "--general",
		shimSendTo+"="+worker.name, shimSendText+"="+pendingWakeText, shimInboxJSON+"=1")
	defer stopSession(t, c, waker)
	var report reportView
	found := waitFor(c, 30*time.Second, func() bool {
		for _, message := range readMessages(sender) {
			if message.From == worker.name && message.Kind == "finished" && slices.Equal(message.InReplyTo, []string{task.ID}) {
				report = message
				return true
			}
		}
		return false
	})
	settled := found && waitFor(c, 5*time.Second, func() bool { return len(awaiting(iso, worker)) == 0 })
	out = append(out, finding(obsReportFollows, found && settled && first.Kind == "pending",
		"a finished report settling %s: %v; the task settled afterwards: %v; kinds read by the sender: %v",
		task.ID, found && report.ID != "", settled, kindsFrom(sender, worker)))
	return out
}

// awaiting lists the worker's open obligations as files: one per sender whose
// task it read and has not reported on yet.
func awaiting(iso *Isolation, worker *codexSession) []string {
	found, _ := filepath.Glob(filepath.Join(iso.StateDir, "rooms", "default", "inbox", worker.name, "awaiting", "*", pendingSenderLabel+"-*"))
	return found
}

func kindsFrom(reader, about *codexSession) []string {
	var kinds []string
	for _, message := range readMessages(reader) {
		if message.From == about.name {
			kinds = append(kinds, message.Kind)
		}
	}
	return kinds
}

func lineWith(text, part string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, part) {
			return line
		}
	}
	return ""
}

// The controls: product mutants, each naming the observations it must break
// and requiring the others to hold.

// The turn end ignores the mark: the first turn end is the report again.
var mutantPendingIgnored = mutation{
	name:  "pending-ignored",
	file:  "internal/cli/turnended.go",
	edits: []edit{{"\tif event.Failed || event.Stopped || event.Ended == 0 {\n", "\tif true || event.Failed || event.Stopped || event.Ended == 0 {\n"}},
}

// The interim turn end settles the task anyway: its journal clears the waits
// it reported to.
var mutantPendingSettles = mutation{
	name:  "pending-settles",
	file:  "internal/cli/turn_reports.go",
	edits: []edit{{"\tif !event.Stopped && !event.Pending {\n\t\tjournal.Clear = reported\n", "\tif !event.Stopped {\n\t\tjournal.Clear = reported\n"}},
}

// The interim turn end carries the mark's line alone, as before: whatever the
// turn itself said is lost.
var mutantPendingTextDropped = mutation{
	name:  "pending-text-dropped",
	file:  "internal/cli/turn_reports.go",
	edits: []edit{{"\t\t\ttext += \"\\n\\n\" + turn\n", "\t\t\t_ = turn\n"}},
}

func TestAnIgnoredPendingMarkFails(t *testing.T) {
	runPendingControl(t, mutantPendingIgnored, obsInterimArrives, obsTaskStillOwed, obsReportFollows)
}

func TestAPendingTurnEndThatSettlesFails(t *testing.T) {
	runPendingControl(t, mutantPendingSettles, obsTaskStillOwed, obsReportFollows)
}

func TestAPendingTurnEndWithoutItsTextFails(t *testing.T) {
	runPendingControl(t, mutantPendingTextDropped, obsInterimArrives)
}

func runPendingControl(t *testing.T, mutant mutation, breaks ...string) {
	t.Helper()
	runInColumns(t, "pending-report-control-"+mutant.name, func(t *testing.T, col column) {
		name := "pending-report-control-" + mutant.name
		enterScenario(t, name)
		want := "the " + mutant.name + " mutant breaks " + strings.Join(breaks, "; ") + ", and nothing else"
		c := Start(t, Spec{Name: name, Harness: col.harness, Observations: []string{want}, Deadline: 150 * time.Second})
		binary, err := buildMutant(c, mutant)
		if err != nil {
			c.Contradicted(want, "the mutant could not be built: %v", err)
			return
		}
		var wrong, broke []string
		for _, finding := range playPendingReport(t, c, Isolate(t, c, binary), col) {
			expected := slices.Contains(breaks, finding.observation)
			switch {
			case !finding.judged:
				wrong = append(wrong, "could not judge "+finding.observation+": "+finding.detail)
			case expected && finding.held:
				wrong = append(wrong, finding.observation+" held anyway: "+finding.detail)
			case !expected && !finding.held:
				wrong = append(wrong, finding.observation+" broke too: "+finding.detail)
			case expected:
				broke = append(broke, finding.observation)
			}
		}
		if len(wrong) == 0 && len(broke) != len(breaks) {
			wrong = append(wrong, fmt.Sprintf("%d of the %d named observations were made", len(broke), len(breaks)))
		}
		if len(wrong) > 0 {
			c.Contradicted(want, "%s", strings.Join(wrong, "; "))
			return
		}
		c.Observed(want, strings.Join(broke, "; "))
	})
}
