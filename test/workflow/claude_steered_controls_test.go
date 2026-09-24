package workflow

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

// The controls. Each breaks one link of a request's path, in the product, and
// the observations resting on that link go with it.

// The module takes a compaction and never asks the host for it.
var mutantCompactNotRun = mutation{
	name: "compact-not-run",
	file: "internal/harness/claude/plugin.js",
	edits: []edit{{
		`const result = focus === "" ? await $.session.compact() : await $.session.compact({ instructions: focus })`,
		`const result = {}`,
	}},
}

// The module does not know the host's mid-turn refusal.
var mutantInTurnUnmapped = mutation{
	name:  "in-turn-unmapped",
	file:  "internal/harness/claude/plugin.js",
	edits: []edit{{`["a turn is running", "in a turn"]`, `["a turn is never running", "in a turn"]`}},
}

// The module aborts the turn without saying who asked.
var mutantInterrupterUnnamed = mutation{
	name:  "interrupter-unnamed",
	file:  "internal/harness/claude/plugin.js",
	edits: []edit{{"aborting = { turn: running, by: word(asked.from) }", "aborting = { turn: running, by: undefined }"}},
}

// The mark is never used up and never laid aside: every notice says the turn
// was interrupted. Both guards go together because either one alone keeps the
// line to one notice here: the next turn's start lays the mark aside before a
// second notice is composed, and a notice composed between a delivery and its
// turn's start, where only Told stops it, is left to the unit tests.
var mutantLineRepeated = mutation{
	name: "line-repeated",
	file: "internal/harness/claude/telemetry/collector_turns.go",
	edits: []edit{
		{"if mark == c.interrupts {", "if false && mark == c.interrupts {"},
		{"case event.Kind == TurnStart, event.Kind == TurnComplete:", "case false:"},
	},
}

// The module answers an interrupt of an idle session as done.
var mutantIdleInterruptDone = mutation{
	name:  "idle-interrupt-done",
	file:  "internal/harness/claude/plugin.js",
	edits: []edit{{`if (turn === undefined) return { outcome: "refused", reason: "no turn running" }`, `if (turn === undefined) return { outcome: "done" }`}},
}

// A request nobody took is reported as a failure, not as not answering.
var mutantSilentNotAnswering = mutation{
	name:  "silent-not-answering",
	file:  "internal/control/control.go",
	edits: []edit{{"Outcome: Refused, Reason: NotAnswering, Detail: detail}", "Outcome: Failed, Detail: detail}"}},
}

// Any session may steer another.
var mutantAnyRoleSteers = mutation{
	name:  "any-role-steers",
	file:  "internal/cli/steer.go",
	edits: []edit{{"if self.Role != role.Main.ID {", "if false && self.Role != role.Main.ID {"}},
}

// main's wrapper announces a compaction main asked for itself.
var mutantOwnCompactionAnnounced = mutation{
	name:  "own-compaction-announced",
	file:  "internal/wrap/session_notices.go",
	edits: []edit{{"if event.RequestedBy == self.Name {", "if false && event.RequestedBy == self.Name {"}},
}

// The command answers without the session's compaction count.
var mutantCompactionUncounted = mutation{
	name:  "compaction-uncounted",
	file:  "internal/cli/steer.go",
	edits: []edit{{"model.Compaction = compactionCount(asking, dir, session, answer.ID)", "model.Compaction = nil"}},
}

// The module compacts without telling rewake who asked: the compaction is
// nobody's, so main gets a notice and the command finds no count.
var mutantAskerUntold = mutation{
	name:  "asker-untold",
	file:  "internal/harness/claude/plugin.js",
	edits: []edit{{`if (announced) await told($, { plugin_event: "compact.asked"`, `if (false) await told($, { plugin_event: "compact.asked"`}},
}

// main's list of what it is owed puts every stop down to a person.
var mutantStopByAPerson = mutation{
	name:  "stop-by-a-person",
	file:  "internal/cli/inbox_awaited.go",
	edits: []edit{{`return detail("stopped")`, `return "stopped by a person"`}},
}

func TestACompactionNeverAskedOfTheHostFails(t *testing.T) {
	runSteeredControl(t, mutantCompactNotRun, obsCompactIdle, obsCompactQuiet, obsCompactBusy)
}

func TestMainNoticedOfItsOwnCompactionFails(t *testing.T) {
	runSteeredControl(t, mutantOwnCompactionAnnounced, obsCompactQuiet)
}

func TestACompactionAnsweredWithoutItsCountFails(t *testing.T) {
	runSteeredControl(t, mutantCompactionUncounted, obsCompactQuiet)
}

func TestACompactionNotTiedToItsAskerFails(t *testing.T) {
	runSteeredControl(t, mutantAskerUntold, obsCompactQuiet)
}

func TestAnInterruptListedAsAPersonsStopFails(t *testing.T) {
	runSteeredControl(t, mutantStopByAPerson, obsInterruptBusy)
}

func TestAnUnmappedMidTurnRefusalFails(t *testing.T) {
	runSteeredControl(t, mutantInTurnUnmapped, obsCompactBusy)
}

func TestAnInterruptThatDoesNotNameMainFails(t *testing.T) {
	runSteeredControl(t, mutantInterrupterUnnamed, obsInterruptBusy, obsInterruptLine)
}

func TestAnInterruptLineRepeatedFails(t *testing.T) {
	runSteeredControl(t, mutantLineRepeated, obsInterruptLine)
}

func TestAnIdleInterruptAnsweredDoneFails(t *testing.T) {
	runSteeredControl(t, mutantIdleInterruptDone, obsInterruptIdle)
}

func TestANotAnsweringReportedAsFailedFails(t *testing.T) {
	runSteeredControl(t, mutantSilentNotAnswering, obsNotAnswering)
}

func TestAWorkerThatSteersFails(t *testing.T) {
	runSteeredControl(t, mutantAnyRoleSteers, obsNotMain)
}

// runSteeredControl is runFindingsControl, unsupported without node like the
// interrupted turn's.
func runSteeredControl(t *testing.T, mutant mutation, breaks ...string) {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		name := "claude-steered-control-" + mutant.name
		enterScenario(t, name)
		want := "the " + mutant.name + " mutant breaks " + strings.Join(breaks, "; ") + ", and nothing else"
		c := Start(t, Spec{Name: name, Harness: claudeColumn.harness, Observations: []string{want}, Deadline: time.Minute})
		c.UnsupportedCapability(want, "node", "no node on PATH to run the plugin's module")
		return
	}
	runFindingsControl(t, "claude-steered", playSteered, mutant, breaks...)
}
