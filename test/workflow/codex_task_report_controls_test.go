package workflow

import (
	"os"
	"slices"
	"testing"
)

// The delivery scenario's negative controls, in both columns. Each one breaks
// exactly one thing and must take down the observation that covers it —
// otherwise the scenario would pass on a fixture and a wrapper agreeing with
// each other.
//
// Four break the fixture: a session that records the wrong message id, one
// whose read fails, one whose turn fails after its answer, one that leaves.
// Three break the product, where the product is reachable: the Stop hook left
// out of the launch settings, `rewake turn-ended` not taking a Stop as the end
// of a turn, and a report that settles nothing. Two of those three exist only
// on the socket column — the other column's reports do not travel through a
// hook or through turn-ended — and a control that cannot break anything on a
// column would pass there for the wrong reason, so it is not run there.

// The observations the controls name. Four are the scenario's own; the fifth,
// that the report is a finished answer, is how the scenario's "the report
// reaches the sender" is told apart from an error report arriving instead.
const (
	obsTRRead       = "the recipient read its own mailbox"
	obsTRReaches    = "the report reaches the sender"
	obsTRFinished   = "the report is a finished answer"
	obsTRCorrelates = "the report corresponds to the message that was delivered"
	obsTRAlive      = "both sessions are still running when the case is judged"
)

// reportKind is what a control's world sends back to the sender: nothing, an
// error, or a finished answer. Declared per control and checked on every run
// of it, because the crosswise check skips the cells an observation cannot be
// asked in, and a declaration nobody checked would decide that silently.
type reportKind string

const (
	reportsNothing  reportKind = "nothing"
	reportsError    reportKind = "error"
	reportsFinished reportKind = "finished"
)

var (
	// The Stop hook is what reports a turn that answered, and only a silent
	// role goes without one. Left out for every role, a general session's
	// answer reaches nobody.
	mutantNoStopHook = mutation{
		name:  "no-stop-hook",
		file:  "internal/harness/claude/settings.go",
		edits: []edit{{"if !silent {", "if false {"}},
	}
	// turn-ended takes a Stop as the end of a turn. Made to accept only a
	// failure, a turn that answered ends without anyone being told.
	mutantTurnEndedIgnoresStop = mutation{
		name: "turn-ended-ignores-stop",
		file: "internal/cli/turn_payload.go",
		edits: []edit{{
			`if hook != "" && hook != "Stop" && hook != "StopFailure" {`,
			`if hook != "" && hook != "StopFailure" {`,
		}},
	}
	// A report names the messages it settles. Emptied, it arrives and settles
	// nothing — which is the report the correlation observation exists to
	// catch, and one a check that only asked whether a report came back would
	// call a pass.
	mutantSettlesNothing = mutation{
		name:  "settles-nothing",
		file:  "internal/cli/turn_reports.go",
		edits: []edit{{"InReplyTo: waiter.Messages,", "InReplyTo: nil,"}},
	}
)

// taskReportControl is one control: how its world differs, what that world
// sends back, and which observation it must take down.
type taskReportControl struct {
	name    string
	shim    []string
	mutant  *mutation
	reports reportKind
	// leaves is true for the world where the recipient goes away on its own
	// after a turn. That departure is what the run waits for before judging:
	// judged the moment the report arrived, the session had not finished
	// leaving yet and the control passed on timing.
	leaves   bool
	expected string
	// columns it runs in. A product mutant of the socket path cannot break
	// anything on the other column, and would pass there for the wrong reason.
	columns []column
}

var bothColumns = []column{codexColumn, claudeColumn}

var taskReportControls = []taskReportControl{
	{name: "wrong-report", shim: []string{shimWrongReportID + "=1"}, reports: reportsFinished, expected: obsTRCorrelates, columns: bothColumns},
	{name: "read-fails", shim: []string{shimReadFails + "=1"}, reports: reportsNothing, expected: obsTRRead, columns: bothColumns},
	{name: "failure-before-report", shim: []string{shimLateFailure + "=1"}, reports: reportsError, expected: obsTRFinished, columns: bothColumns},
	{name: "early-exit", shim: []string{shimExitAfterTurn + "=1"}, reports: reportsFinished, leaves: true, expected: obsTRAlive, columns: bothColumns},
	{name: "no-stop-hook", mutant: &mutantNoStopHook, reports: reportsNothing, expected: obsTRReaches, columns: []column{claudeColumn}},
	{name: "turn-ended-ignores-stop", mutant: &mutantTurnEndedIgnoresStop, reports: reportsNothing, expected: obsTRReaches, columns: []column{claudeColumn}},
	{name: "settles-nothing", mutant: &mutantSettlesNothing, reports: reportsFinished, expected: obsTRCorrelates, columns: bothColumns},
}

func TestReportNotMatchingTheMessageFails(t *testing.T) { failedTaskReport(t, "wrong-report") }
func TestAFailedMailboxReadFails(t *testing.T)          { failedTaskReport(t, "read-fails") }

// The failure falls between the text of the turn and the publication of its
// report, and the name says so: a failure after publication is a different
// question, see the boundary in check-runner-scenarios.md.
func TestAFailureBeforeTheReportIsPublished(t *testing.T) {
	failedTaskReport(t, "failure-before-report")
}
func TestASessionThatLeavesEarlyFails(t *testing.T) { failedTaskReport(t, "early-exit") }
func TestALaunchWithoutAStopHookFails(t *testing.T) { failedTaskReport(t, "no-stop-hook") }
func TestATurnEndThatIgnoresStopFails(t *testing.T) { failedTaskReport(t, "turn-ended-ignores-stop") }
func TestAReportSettlingNothingFails(t *testing.T)  { failedTaskReport(t, "settles-nothing") }

// runsIn reports whether this control applies to a column. By harness name:
// a column carries its capability table, and a table is not something to
// compare for equality.
func (control taskReportControl) runsIn(col column) bool {
	for _, one := range control.columns {
		if one.harness == col.harness {
			return true
		}
	}
	return false
}

func controlNamed(name string) taskReportControl {
	for _, control := range taskReportControls {
		if control.name == name {
			return control
		}
	}
	panic("no task-report control named " + name)
}

// failedTaskReport runs one control in every column it applies to and requires
// the observation it names to be the one that breaks.
func failedTaskReport(t *testing.T, name string) {
	t.Helper()
	control := controlNamed(name)
	runParallel(t)
	for _, col := range control.columns {
		t.Run(col.harness, func(t *testing.T) {
			enterScenario(t, "task-report-control-"+name)
			runTaskReportControl(t, col, "task-report-control-"+name, control, control.expected, mustBreak)
		})
	}
}

// TestTaskReportControlsCrosswise checks each control's observation against the
// other controls' worlds, per column, and requires it to stand there.
//
// Some cells are not asked, by a rule rather than a list: an observation about
// a report cannot be asked in a world that sends none, and one about a
// finished report not in a world that sends an error. Which world sends what is
// each control's own declaration, and every run of that control checks the
// declaration against what actually came back — so a wrong one turns the
// control red rather than quietly removing a cell here.
func TestTaskReportControlsCrosswise(t *testing.T) {
	if os.Getenv(crossSwitch) == "" {
		t.Skipf("crosswise check skipped: set %s=1 to run every control against every other world", crossSwitch)
	}
	runParallel(t)
	for _, col := range bothColumns {
		t.Run(col.harness, func(t *testing.T) {
			enterScenarioAround(t, "task-report-crosswise")
			for _, observation := range taskReportObservations(col) {
				for _, other := range taskReportControls {
					if other.expected == observation || !other.runsIn(col) || !askable(observation, other.reports) {
						continue
					}
					name := shortTaskObservation(observation) + "-under-" + other.name
					t.Run(name, func(t *testing.T) {
						joinPool(t, "task-report-cross-"+name)
						runTaskReportControl(t, col, "task-report-cross-"+name, other, observation, mustHold)
					})
				}
			}
		})
	}
}

// taskReportObservations are the observations some control breaks in this
// column, each once.
func taskReportObservations(col column) []string {
	var out []string
	for _, control := range taskReportControls {
		if control.runsIn(col) && !slices.Contains(out, control.expected) {
			out = append(out, control.expected)
		}
	}
	return out
}

// askable says whether an observation can be asked in a world that sends
// back this kind of report.
func askable(observation string, reports reportKind) bool {
	switch observation {
	case obsTRReaches:
		// Asked where the task was read and an answer is owed: a world that
		// sends nothing because nothing was read has no report to miss.
		return reports != reportsNothing
	case obsTRFinished:
		return reports != reportsNothing
	case obsTRCorrelates:
		return reports == reportsFinished
	}
	return true
}

func shortTaskObservation(observation string) string {
	switch observation {
	case obsTRRead:
		return "read"
	case obsTRReaches:
		return "reaches"
	case obsTRFinished:
		return "finished"
	case obsTRCorrelates:
		return "correlates"
	case obsTRAlive:
		return "alive"
	}
	return "observation"
}
