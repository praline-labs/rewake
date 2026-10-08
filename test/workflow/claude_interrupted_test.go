package workflow

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestClaudeInterruptedTurn is rewake's function-hooks plugin end to end on
// the Claude Code column: a main hands a task to four workers.
//
//   - One is interrupted in its first turn, the way a person's Esc ends it:
//     the turn reads the task and ends with turn.complete reason "aborted" and
//     no Stop hook. Its plugin reports that, so main hears stopped at once and
//     still waits; woken by a note, the worker's next turn ends in Stop, and
//     that finished settles the task, as on the fixture.
//   - One works an ordinary turn: the plugin hears its end as well, and main
//     still reads exactly one finished.
//   - One is interrupted just as its turn ends: the Stop hook has run and
//     reported, and the turn still ends with turn.complete "aborted", the
//     order seen live. The plugin heard the Stop and keeps quiet, so main
//     reads one finished and no stopped.
//   - One is interrupted in a session whose harness did not load the plugin:
//     nothing is heard, the listing says interruptions are unobserved, and the
//     next finished settles the task — what rewake did before the plugin.
//
// main runs every command itself, as in the awaited view: what is judged is
// what main's own rewake shows.
//
// What it does not prove: that the real harness calls the plugin when it says
// it does. That was seen live and is recorded in
// docs/research-claude-control.md; the fixture runs the module rewake wrote
// under node and plays those events to it.
func TestClaudeInterruptedTurn(t *testing.T) {
	binary := enterScenario(t, "claude-interrupted")
	c := Start(t, Spec{
		Name:         "claude-interrupted",
		Harness:      claudeColumn.harness,
		Observations: interruptedObservations,
		Deadline:     120 * time.Second,
	})
	if _, err := exec.LookPath("node"); err != nil {
		for _, observation := range interruptedObservations {
			c.UnsupportedCapability(observation, "node", "no node on PATH to run the plugin's module")
		}
		return
	}
	for _, finding := range playInterruptedTurn(t, c, Isolate(t, c, binary)) {
		if finding.held {
			c.Observed(finding.observation, finding.detail)
		} else {
			c.Contradicted(finding.observation, "%s", finding.detail)
		}
	}
}

const (
	obsStoppedAtOnce     = "an interrupted turn reaches main as stopped before any later turn, and the task stays awaited"
	obsInterruptedIdle   = "the interrupted worker reads idle, with interruptions observed"
	obsStoppedThenSettle = "the next finished turn end settles the interrupted task: stopped, then exactly one finished, and nothing more, one stop published"
	obsOrdinaryOnce      = "an ordinary turn heard by the plugin reports exactly one finished, and publishes no stop"
	obsUnheardAsBefore   = "without the plugin nothing is heard, interruptions read unobserved, and the next finished settles the task"
	obsLateEscOnce       = "an Esc landing after the Stop hook gives exactly one finished, and no stopped, published or sent"
	interruptedTask      = "claude-interrupted: run the long check"
	interruptedWake      = "claude-interrupted-wake: go on"
)

var interruptedObservations = []string{
	obsStoppedAtOnce, obsInterruptedIdle, obsStoppedThenSettle, obsOrdinaryOnce, obsUnheardAsBefore, obsLateEscOnce,
}

// interruptedRow is the part of a listing's telemetry this scenario reads.
type interruptedRow struct {
	Activity      string `json:"activity"`
	Interruptions string `json:"interruptions"`
}

func playInterruptedTurn(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		var out []telemetryFinding
		for _, observation := range interruptedObservations {
			out = append(out, telemetryFinding{observation: observation, detail: "no node to run the plugin's module"})
		}
		return out
	}
	runs := shimNode + "=" + node
	// The workers are asked nothing, but the wake comes late in the case: a
	// slower control, or a heads-up waiting for company, puts it past the
	// ordinary ceiling.
	heard := startHarnessSession(t, c, iso, claudeColumn.harness, "heard", "--general", shimInboxJSON+"=1", shimInterruptFirst+"=1", runs, staysUp(iso, "heard"))
	defer stopSession(t, c, heard)
	ordinary := startHarnessSession(t, c, iso, claudeColumn.harness, "ordinary", "--general", shimInboxJSON+"=1", runs, staysUp(iso, "ordinary"))
	defer stopSession(t, c, ordinary)
	late := startHarnessSession(t, c, iso, claudeColumn.harness, "late", "--general", shimInboxJSON+"=1", shimInterruptAtStop+"=1", runs, staysUp(iso, "late"))
	defer stopSession(t, c, late)
	unheard := startHarnessSession(t, c, iso, claudeColumn.harness, "unheard", "--general", shimInboxJSON+"=1",
		shimInterruptFirst+"=1", shimNoFunctionHooks+"=1", runs, staysUp(iso, "unheard"))
	defer stopSession(t, c, unheard)
	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, claudeColumn.harness, "lead", "--main", shimInboxJSON+"=1", asks.env())
	defer stopSession(t, c, lead)

	finding := func(observation string, held bool, detail string, args ...any) telemetryFinding {
		return telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)}
	}
	unjudged := func(detail string) []telemetryFinding {
		var out []telemetryFinding
		for _, observation := range interruptedObservations {
			out = append(out, telemetryFinding{observation: observation, detail: detail})
		}
		return out
	}
	// The kinds main read from a worker about one task, in the order read.
	about := func(worker *scenarioSession, task string) []string {
		var kinds []string
		for _, message := range readMessages(lead) {
			if message.From == worker.name && slices.Contains(message.InReplyTo, task) {
				kinds = append(kinds, message.Kind)
			}
		}
		return kinds
	}
	// Every report main read from a worker, whatever it names. The
	// availability notice comes from the worker too and answers nothing.
	reports := func(worker *scenarioSession) []string {
		var kinds []string
		for _, kind := range kindsFrom(lead, worker) {
			if kind != "notify" {
				kinds = append(kinds, kind)
			}
		}
		return kinds
	}
	// The state main's own rewake gives a task in its awaited view, or "" when
	// the view does not list it.
	awaited := func(task string) (string, string) {
		code, machine, ok := asks.ask(c, "inbox", "--awaited", "--json")
		var model awaitedModelView
		if !ok || code != 0 || json.Unmarshal([]byte(machine), &model) != nil {
			return "", fmt.Sprintf("the view did not come back: exit %d, %q", code, machine)
		}
		for _, recipient := range model.Recipients {
			for _, message := range recipient.Messages {
				if message.ID == task {
					return message.State, ""
				}
			}
		}
		return "", ""
	}
	// The stops a worker published. A stop after the finished that settled
	// every wait goes to nobody, so main's inbox cannot show it; the journal
	// the worker keeps for it can. On this column only a stop leaves one named
	// by its event: the Stop hook's report names no turn, and its journal's
	// name is drawn.
	stops := func(worker *scenarioSession) int { return eventEnds(iso, worker.name) }
	// A worker's row in main's listing, as main's own rewake printed it.
	row := func(worker *scenarioSession) (interruptedRow, string) {
		code, machine, ok := asks.ask(c, "list", "--json")
		var listed struct {
			Sessions []struct {
				Name      string         `json:"name"`
				Telemetry interruptedRow `json:"telemetry"`
			} `json:"sessions"`
		}
		if !ok || code != 0 || json.Unmarshal([]byte(machine), &listed) != nil {
			return interruptedRow{}, fmt.Sprintf("the listing did not come back: exit %d, %q", code, firstLine(machine))
		}
		for _, entry := range listed.Sessions {
			if entry.Name == worker.name {
				return entry.Telemetry, ""
			}
		}
		return interruptedRow{}, worker.name + " is not listed"
	}

	var sends []string
	workers := []*scenarioSession{heard, ordinary, late, unheard}
	for _, worker := range workers {
		code, out, _ := asks.ask(c, "send", worker.name, interruptedTask)
		sends = append(sends, fmt.Sprintf("to %s: exit %d, %s", worker.name, code, firstLine(out)))
	}
	tasks := map[*scenarioSession]string{}
	if !waitFor(c, 30*time.Second, func() bool {
		for _, worker := range workers {
			message, read := messageCarrying(worker, "claude-interrupted:")
			if !read {
				return false
			}
			tasks[worker] = message.ID
		}
		return true
	}) {
		return unjudged(fmt.Sprintf("not every worker read its task; sends: %v", sends))
	}

	var out []telemetryFinding
	// Stopped, before anything else happens on the heard worker: no later
	// turn has been asked of it yet, so whatever reaches main now came from
	// the interruption itself.
	begun := time.Now()
	// A ceiling the healthy run ends within a second or two — the report waits
	// up to suiteCap for company — and the controls that publish nothing wait
	// out in full.
	stoppedHeard := waitFor(c, suiteCap+3*time.Second, func() bool { return slices.Contains(about(heard, tasks[heard]), "stopped") })
	state, failure := awaited(tasks[heard])
	out = append(out, finding(obsStoppedAtOnce, stoppedHeard && state == "stopped" && failure == "",
		"after %s main read %v about the task; the awaited view gives it %q %s", time.Since(begun).Round(10*time.Millisecond), about(heard, tasks[heard]), state, failure))

	// The single reports of the ordinary and the late worker are in by now or
	// soon; a second one, if the plugin's end were counted too, would come in
	// the same moment.
	waitFor(c, 15*time.Second, func() bool {
		return slices.Contains(about(ordinary, tasks[ordinary]), "finished") && slices.Contains(about(late, tasks[late]), "finished")
	})
	time.Sleep(2 * time.Second)
	kinds, published := reports(ordinary), stops(ordinary)
	out = append(out, finding(obsOrdinaryOnce, slices.Equal(kinds, []string{"finished"}) && published == 0,
		"main read %v from the ordinary worker, its availability notice aside; it published %d stops", kinds, published))
	kinds, published = reports(late), stops(late)
	out = append(out, finding(obsLateEscOnce, slices.Equal(kinds, []string{"finished"}) && published == 0,
		"main read %v from the worker interrupted after its Stop hook, its availability notice aside; it published %d stops", kinds, published))

	heardRow, failure := row(heard)
	unheardRow, unheardFailure := row(unheard)
	out = append(out, finding(obsInterruptedIdle, failure == "" && heardRow.Activity == "idle" && heardRow.Interruptions == "observed",
		"activity %q, interruptions %q %s", heardRow.Activity, heardRow.Interruptions, failure))

	// The wake: a note starts the next turn, which ends in Stop.
	for _, worker := range []*scenarioSession{heard, unheard} {
		code, sent, _ := asks.ask(c, "send", worker.name, interruptedWake, "--notify")
		sends = append(sends, fmt.Sprintf("note to %s: exit %d, %s", worker.name, code, firstLine(sent)))
	}
	settled := func(worker *scenarioSession) bool { return slices.Contains(about(worker, tasks[worker]), "finished") }
	waitFor(c, 30*time.Second, func() bool { return settled(heard) && settled(unheard) })
	// Anything a mutant adds after the finished arrives beside it.
	time.Sleep(time.Second)
	heardState, failure := awaited(tasks[heard])
	heardKinds, heardAll, published := about(heard, tasks[heard]), reports(heard), stops(heard)
	out = append(out, finding(obsStoppedThenSettle,
		slices.Equal(heardKinds, []string{"stopped", "finished"}) && slices.Equal(heardAll, heardKinds) && published == 1 && heardState == "" && failure == "",
		"main read %v about the task and %v from the worker in all, which published %d stops over two turns; the awaited view gives it %q %s; sends: %v",
		heardKinds, heardAll, published, heardState, failure, sends))

	unheardState, failure := awaited(tasks[unheard])
	unheardKinds := about(unheard, tasks[unheard])
	out = append(out, finding(obsUnheardAsBefore,
		unheardFailure == "" && unheardRow.Interruptions == "unobserved" && slices.Equal(unheardKinds, []string{"finished"}) && unheardState == "" && failure == "",
		"before the note: activity %q, interruptions %q %s; after it main read %v about the task, the awaited view gives it %q %s",
		unheardRow.Activity, unheardRow.Interruptions, unheardFailure, unheardKinds, unheardState, failure))
	return out
}

// The controls. Each breaks one link of the path from the harness's event to
// main's inbox, and the observations that rest on that link go with it.

// The collector hears an interrupted turn and does nothing with it.
var mutantInterruptUnpublished = mutation{
	name:  "interrupt-unpublished",
	file:  "internal/harness/claude/telemetry/collector.go",
	edits: []edit{{"event.Reason == ReasonAborted", "event.Reason == \"never\""}},
}

// The collector takes every turn end for an interruption: an ordinary one is
// then reported twice, by the plugin and by the Stop hook.
var mutantEveryEndStopped = mutation{
	name:  "every-end-stopped",
	file:  "internal/harness/claude/telemetry/collector.go",
	edits: []edit{{"event.Reason == ReasonAborted", "event.Reason != \"\""}},
}

// The plugin does not take the Stop hook into account: an Esc landing after
// it reports the reported turn again, as stopped.
var mutantStopNotHeard = mutation{
	name:  "stop-not-heard",
	file:  "internal/harness/claude/plugin.js",
	edits: []edit{{"reason === \"aborted\" && hooked", "reason === \"aborted\" && false"}},
}

// The launch never carries the plugin.
var mutantPluginNotPassed = mutation{
	name:  "plugin-not-passed",
	file:  "internal/harness/claude/plugin.go",
	edits: []edit{{"\tif socket == \"\" {\n", "\tif true {\n"}},
}

func TestAnUnpublishedInterruptionFails(t *testing.T) {
	runInterruptedControl(t, mutantInterruptUnpublished, obsStoppedAtOnce, obsStoppedThenSettle)
}

func TestTakingEveryTurnEndForAnInterruptionFails(t *testing.T) {
	runInterruptedControl(t, mutantEveryEndStopped, obsStoppedThenSettle, obsOrdinaryOnce)
}

func TestALaunchWithoutThePluginFails(t *testing.T) {
	runInterruptedControl(t, mutantPluginNotPassed, obsStoppedAtOnce, obsInterruptedIdle, obsStoppedThenSettle)
}

func TestAnEscAfterTheStopHookReportedTwiceFails(t *testing.T) {
	runInterruptedControl(t, mutantStopNotHeard, obsLateEscOnce)
}

// runInterruptedControl is runFindingsControl, except that without node the
// control is unsupported rather than unjudged: nothing runs the module, and
// every observation would break for that reason alone.
func runInterruptedControl(t *testing.T, mutant mutation, breaks ...string) {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		name := "claude-interrupted-control-" + mutant.name
		enterScenario(t, name)
		want := "the " + mutant.name + " mutant breaks " + strings.Join(breaks, "; ") + ", and nothing else"
		c := Start(t, Spec{Name: name, Harness: claudeColumn.harness, Observations: []string{want}, Deadline: time.Minute})
		c.UnsupportedCapability(want, "node", "no node on PATH to run the plugin's module")
		return
	}
	runFindingsControl(t, "claude-interrupted", playInterruptedTurn, mutant, breaks...)
}

// eventEnds counts the turn ends a session acted on that were named by their
// event: each leaves a journal named from its run and event, 32 hex digits,
// kept while the run lives; one heard once without an event has a drawn name
// (docs/turn-end-recovery.md#the-operation).
func eventEnds(iso *Isolation, name string) int {
	found, _ := filepath.Glob(filepath.Join(iso.StateDir, "rooms", "default", "inbox", name, "journal", "*"))
	count := 0
	for _, path := range found {
		if eventNamed.MatchString(strings.TrimSuffix(filepath.Base(path), ".done")) {
			count++
		}
	}
	return count
}

var eventNamed = regexp.MustCompile(`^[0-9a-f]{32}$`)
