package workflow

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestPendingConfirm is the Stop hook's confirmation on the Claude Code column.
// A worker reads a task and ends its first turn pending. Woken by another
// session's mail, its next turn ends with no mark — the turn a finished
// subagent starts, where the mark is easily forgotten — and rewake's Stop hook
// holds it once, quoting the pending line. The fixture's model, asked, marks
// pending: the sender reads an interim message with the new line, then the
// held answer, then the continuation, and the task stays owed. Woken again,
// the worker ends a turn with no mark once more, is held once more, and this
// time only ends the turn: the sender reads the report, the held answer first
// and the continuation after it, and the task settles.
//
// What it does not prove: that the real harness honors a block from rewake's
// settings layer and calls the hook again as the fixture does. That was seen
// live on 2.1.280 and is recorded in docs/research.md.
func TestPendingConfirm(t *testing.T) {
	binary := enterScenario(t, "pending-confirm")
	c := Start(t, Spec{
		Name:         "pending-confirm",
		Harness:      claudeColumn.harness,
		Observations: confirmObservations,
		Deadline:     90 * time.Second,
	})
	for _, finding := range playPendingConfirm(t, c, Isolate(t, c, binary)) {
		if finding.held {
			c.Observed(finding.observation, finding.detail)
		} else {
			c.Contradicted(finding.observation, "%s", finding.detail)
		}
	}
}

const (
	obsConfirmAsked   = "the unmarked turn end after an interim one is held exactly once, and the hold quotes the pending line"
	obsConfirmMarked  = "a continuation that marks pending sends an interim message: the new line, the held answer, then the continuation; the task stays owed"
	obsConfirmReports = "the next unmarked turn end is held once again, and its continuation's end is the report: the held answer, then the continuation; the task settles"
	confirmTaskText   = "pending-confirm: run the suite and report"
	confirmFirstLine  = "the suite is running in the background"
	confirmSecondLine = "the second half of the suite is running"
	confirmWakeOne    = "pending-confirm-wake-one: the first half finished"
	confirmWakeTwo    = "pending-confirm-wake-two: the second half finished"
)

var confirmObservations = []string{obsConfirmAsked, obsConfirmMarked, obsConfirmReports}

func playPendingConfirm(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	t.Helper()
	col := claudeColumn
	worker := startHarnessSession(t, c, iso, col.harness, "worker", "--general", shimInboxJSON+"=1",
		shimPendingOnce+"="+confirmFirstLine, shimPendingOnHold+"="+confirmSecondLine)
	defer stopSession(t, c, worker)
	sender := startHarnessSession(t, c, iso, col.harness, pendingSenderLabel, "--main",
		shimSendTo+"="+worker.name, shimSendText+"="+confirmTaskText, shimInboxJSON+"=1", readinessSwitch(col, worker))
	defer stopSession(t, c, sender)

	unjudged := func(from int, detail string) []telemetryFinding {
		var out []telemetryFinding
		for _, observation := range confirmObservations[from:] {
			out = append(out, telemetryFinding{observation: observation, detail: detail})
		}
		return out
	}
	finding := func(observation string, held bool, detail string, args ...any) telemetryFinding {
		return telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)}
	}
	// fromWorker is what the sender has read from the worker about the task,
	// in order.
	fromWorker := func(task string) []reportView {
		var out []reportView
		for _, message := range readMessages(sender) {
			if message.From == worker.name && slices.Contains(message.InReplyTo, task) {
				out = append(out, message)
			}
		}
		return out
	}
	holds := func() []string {
		var out []string
		for _, line := range strings.Split(worker.acceptedTurns(), "\n") {
			if _, reason, ok := strings.Cut(line, " held: "); ok {
				out = append(out, reason)
			}
		}
		return out
	}

	var task reportView
	if !waitFor(c, 30*time.Second, func() bool {
		var ok bool
		task, ok = messageCarrying(worker, confirmTaskText)
		return ok && len(fromWorker(task.ID)) > 0
	}) {
		return unjudged(0, "the worker never ended its first turn about the task: "+worker.acceptedTurns())
	}
	if first := fromWorker(task.ID)[0]; first.Kind != "pending" || len(awaiting(iso, worker)) == 0 {
		return unjudged(0, fmt.Sprintf("the first turn end was not interim, so nothing can be asked: %s %q, turns %s", first.Kind, first.Text, worker.acceptedTurns()))
	}

	// The first wake. No readiness wait: the worker has taken mail already.
	waker := startHarnessSession(t, c, iso, col.harness, "waker", "--general",
		shimSendTo+"="+worker.name, shimSendText+"="+confirmWakeOne, shimInboxJSON+"=1")
	defer stopSession(t, c, waker)
	var second reportView
	if !waitFor(c, 30*time.Second, func() bool {
		messages := fromWorker(task.ID)
		if len(messages) < 2 {
			return false
		}
		second = messages[1]
		return true
	}) {
		return unjudged(0, "the sender read nothing more about the task after the first wake; turns: "+worker.acceptedTurns())
	}
	asked := holds()
	out := []telemetryFinding{finding(obsConfirmAsked, len(asked) == 1 && strings.Contains(asked[0], confirmFirstLine),
		"holds after the first wake: %q", asked)}
	marked := second.Kind == "pending" && strings.HasPrefix(second.Text, confirmSecondLine+"\n\n") &&
		strings.Contains(second.Text, confirmWakeOne) && strings.HasSuffix(second.Text, "\n\n"+holdContinuation)
	owed := len(awaiting(iso, worker)) > 0
	out = append(out, finding(obsConfirmMarked, marked && owed,
		"the second message about the task was %s: %q; still owed: %v", second.Kind, second.Text, owed))
	if !owed {
		return append(out, finding(obsConfirmReports, false, "the task was settled already; kinds read by the sender: %v", kindsFrom(sender, worker)))
	}

	// The second wake: the worker is done this time.
	closer := startHarnessSession(t, c, iso, col.harness, "closer", "--general",
		shimSendTo+"="+worker.name, shimSendText+"="+confirmWakeTwo, shimInboxJSON+"=1")
	defer stopSession(t, c, closer)
	var report reportView
	found := waitFor(c, 30*time.Second, func() bool {
		for _, message := range fromWorker(task.ID) {
			if message.Kind == "finished" {
				report = message
				return true
			}
		}
		return false
	})
	settled := found && waitFor(c, 5*time.Second, func() bool { return len(awaiting(iso, worker)) == 0 })
	all := holds()
	reported := found && settled && !strings.Contains(report.Text, confirmWakeOne) && strings.Contains(report.Text, confirmWakeTwo) &&
		strings.HasSuffix(report.Text, "\n\n"+holdContinuation) &&
		len(all) == 2 && strings.Contains(all[1], confirmSecondLine)
	return append(out, finding(obsConfirmReports, reported,
		"the report %q; settled: %v; holds %q; kinds read by the sender: %v", report.Text, settled, all, kindsFrom(sender, worker)))
}

// The controls: product mutants, each naming the observations it must break
// and requiring the others to hold.

// The Stop hook never holds: the turn woken after an interim one is the report
// at once, and a forgotten mark closes the task early.
var mutantPendingUnconfirmed = mutation{
	name:  "pending-unconfirmed",
	file:  "internal/cli/turn_hold.go",
	edits: []edit{{"\tif !event.Holdable || event.Failed || event.Stopped {\n", "\tif true || !event.Holdable || event.Failed || event.Stopped {\n"}},
}

// The held answer is not published: the report is the continuation alone,
// which is what the harness's second Stop carries.
var mutantConfirmAnswerDropped = mutation{
	name:  "confirm-answer-dropped",
	file:  "internal/cli/turn_reports.go",
	edits: []edit{{"\t\t\t\t\tevent.Text = joinTurnText(held, event.Text)\n", "\t\t\t\t\tevent.Text = joinTurnText(\"\", event.Text)\n"}},
}

func TestAnUnconfirmedPendingFails(t *testing.T) {
	runConfirmControl(t, mutantPendingUnconfirmed, obsConfirmAsked, obsConfirmMarked, obsConfirmReports)
}

func TestAConfirmationThatDropsTheAnswerFails(t *testing.T) {
	runConfirmControl(t, mutantConfirmAnswerDropped, obsConfirmMarked, obsConfirmReports)
}

func runConfirmControl(t *testing.T, mutant mutation, breaks ...string) {
	t.Helper()
	runFindingsControlOn(t, claudeColumn.harness, "pending-confirm", playPendingConfirm, mutant, breaks...)
}
