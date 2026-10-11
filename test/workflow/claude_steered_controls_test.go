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
		`(focus === "" ? $.session.compact() : $.session.compact({ instructions: focus }))`,
		`({})`,
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
	edits: []edit{{`if (turn === undefined) return { answer: { outcome: "refused", reason: "no turn running" } }`, `if (turn === undefined) return { answer: { outcome: "done" } }`}},
}

// The module answers only once the compaction has ended, as the command did
// before it stopped waiting for the end.
var mutantWaitsForTheEnd = mutation{
	name:  "waits-for-the-end",
	file:  "internal/harness/claude/plugin.js",
	edits: []edit{{`const first = await Promise.race([compaction, begun($, asked.id, () => over), bound])`, `const first = await compaction`}},
}

// The module tells rewake a compaction ended without the host's counts.
var mutantEndedUncounted = mutation{
	name:  "ended-uncounted",
	file:  "internal/harness/claude/plugin.js",
	edits: []edit{{`outcome = { outcome: "done", tokensBefore: number(counts.tokensBefore), tokensAfter: number(counts.tokensAfter) }`, `outcome = { outcome: "done" }`}},
}

// The module compacts without telling rewake who asked: the compaction is
// nobody's, so its start is not marked for the module, which answers
// requested, main gets a notice, and the letter finds no count.
var mutantAskerUntold = mutation{
	name:  "asker-untold",
	file:  "internal/harness/claude/plugin.js",
	edits: []edit{{`if (announced) await told($, { plugin_event: "compact.asked"`, `if (false) await told($, { plugin_event: "compact.asked"`}},
}

func TestACompactionNeverAskedOfTheHostFails(t *testing.T) {
	runClaudeSteeredControl(t, mutantCompactNotRun, obsCompactLetter, obsCompactQuiet, obsCompactBusy)
}

func TestACompactionAnsweredAtItsEndFails(t *testing.T) {
	runClaudeSteeredControl(t, mutantWaitsForTheEnd, obsCompactStarted)
}

func TestALetterWithoutTheTokensFails(t *testing.T) {
	runClaudeSteeredControl(t, mutantEndedUncounted, obsCompactLetter)
}

func TestACompactionNotTiedToItsAskerFails(t *testing.T) {
	runClaudeSteeredControl(t, mutantAskerUntold, obsCompactStarted, obsCompactLetter, obsCompactQuiet, obsCompactOrphan)
}

func TestAnUnmappedMidTurnRefusalFails(t *testing.T) {
	runClaudeSteeredControl(t, mutantInTurnUnmapped, obsCompactBusy)
}

func TestAnInterruptThatDoesNotNameMainFails(t *testing.T) {
	runClaudeSteeredControl(t, mutantInterrupterUnnamed, obsInterruptBusy, obsInterruptLine)
}

func TestAnInterruptLineRepeatedFails(t *testing.T) {
	runClaudeSteeredControl(t, mutantLineRepeated, obsInterruptLine)
}

func TestAnIdleInterruptAnsweredDoneFails(t *testing.T) {
	runClaudeSteeredControl(t, mutantIdleInterruptDone, obsInterruptIdle)
}

// runSteeredControl is runFindingsControl, unsupported without node like the
// interrupted turn's.
func runClaudeSteeredControl(t *testing.T, mutant mutation, breaks ...string) {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		name := "claude-steered-control-" + mutant.name
		enterScenario(t, name)
		want := "the " + mutant.name + " mutant breaks " + strings.Join(breaks, "; ") + ", and nothing else"
		c := Start(t, Spec{Name: name, Harness: claudeColumn.harness, Observations: []string{want}, Deadline: time.Minute})
		c.UnsupportedCapability(want, "node", "no node on PATH to run the plugin's module")
		return
	}
	runFindingsControl(t, "claude-steered", playClaudeSteered, mutant, breaks...)
}
