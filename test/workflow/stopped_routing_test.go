package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// TestStoppedRouting is where a stopped goes, on both columns: only to the
// sessions waiting for a report from the interrupted one (the owner's decision
// of September 23, 2026). A main hands one worker a task and another a notify,
// which owes nothing; both workers, and main itself, have their first turn
// interrupted.
//
//   - The worker holding the task reports stopped to main, which still waits.
//   - The worker woken by the notify owes no one: its interruption sends
//     nothing, to main or anyone, and it simply reads idle.
//   - Main's own interrupted turn puts nothing into its own inbox.
//
// On the Claude Code column an interruption is heard through rewake's plugin,
// so without node to run it the case is unsupported there.
func TestStoppedRouting(t *testing.T) {
	runInColumns(t, "stopped-routing", func(t *testing.T, col column) {
		binary := enterScenario(t, "stopped-routing")
		c := Start(t, Spec{
			Name:         "stopped-routing",
			Harness:      col.harness,
			Observations: stoppedRoutingObservations,
			Deadline:     90 * time.Second,
		})
		if !col.offers(capabilitySelection) {
			if _, err := exec.LookPath("node"); err != nil {
				for _, observation := range stoppedRoutingObservations {
					c.UnsupportedCapability(observation, "node", "no node on PATH to run the plugin's module")
				}
				return
			}
		}
		for _, finding := range playStoppedRouting(t, c, Isolate(t, c, binary), col) {
			if finding.held {
				c.Observed(finding.observation, finding.detail)
			} else {
				c.Contradicted(finding.observation, "%s", finding.detail)
			}
		}
	})
}

const (
	obsStoppedToWaiter = "an interrupted turn holding a task reaches its sender as stopped, and the task stays awaited"
	obsStoppedToNobody = "an interrupted turn nobody waits on sends no message, and the worker reads idle"
	obsMainOwnEsc      = "an interrupted turn of main puts nothing into its own inbox"
	stoppedRoutingTask = "stopped-routing: run the long check"
	stoppedRoutingNote = "stopped-routing-note: heads-up"
)

var stoppedRoutingObservations = []string{obsStoppedToWaiter, obsStoppedToNobody, obsMainOwnEsc}

func playStoppedRouting(t *testing.T, c *Case, iso *Isolation, col column) []telemetryFinding {
	t.Helper()
	controls := []string{shimInboxJSON + "=1", shimInterruptFirst + "=1"}
	if !col.offers(capabilitySelection) {
		node, err := exec.LookPath("node")
		if err != nil {
			var out []telemetryFinding
			for _, observation := range stoppedRoutingObservations {
				out = append(out, telemetryFinding{observation: observation, detail: "no node to run the plugin's module"})
			}
			return out
		}
		controls = append(controls, shimNode+"="+node)
	}
	owed := startHarnessSession(t, c, iso, col.harness, "owed", "--general", controls...)
	defer stopSession(t, c, owed)
	free := startHarnessSession(t, c, iso, col.harness, "free", "--general", controls...)
	defer stopSession(t, c, free)
	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, col.harness, "lead", "--main", append(controls, asks.env())...)
	defer stopSession(t, c, lead)

	finding := func(observation string, held bool, detail string, args ...any) telemetryFinding {
		return telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)}
	}
	unjudged := func(detail string) []telemetryFinding {
		var out []telemetryFinding
		for _, observation := range stoppedRoutingObservations {
			out = append(out, telemetryFinding{observation: observation, detail: detail})
		}
		return out
	}
	// The outcomes a session published, one receipt per turn end it acted on:
	// how an interruption that went to nobody is told from one never heard.
	outcomes := func(session *codexSession) int {
		found, _ := filepath.Glob(filepath.Join(iso.StateDir, "rooms", "default", "inbox", session.name, "turns", "*"))
		return len(found)
	}
	// What main holds from a session, read or not: its own mailbox reads and
	// the files still in its inbox. A report main put into its own inbox is
	// not announced, so only the file shows it.
	holds := func(from *codexSession) []string {
		var kinds []string
		for _, message := range readMessages(lead) {
			if message.From == from.name && message.Kind != "notify" {
				kinds = append(kinds, "read "+message.Kind)
			}
		}
		files, _ := filepath.Glob(filepath.Join(iso.StateDir, "rooms", "default", "inbox", lead.name, "*.json"))
		for _, file := range files {
			var message reportView
			raw, err := os.ReadFile(file)
			if err == nil && json.Unmarshal(raw, &message) == nil && message.From == from.name && message.Kind != "notify" {
				kinds = append(kinds, "unread "+message.Kind)
			}
		}
		return kinds
	}
	awaitedState := func(task string) (string, string) {
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
	activity := func(worker *codexSession) string {
		code, machine, ok := asks.ask(c, "list", "--json")
		var listed struct {
			Sessions []struct {
				Name      string `json:"name"`
				Telemetry struct {
					Activity string `json:"activity"`
				} `json:"telemetry"`
			} `json:"sessions"`
		}
		if !ok || code != 0 || json.Unmarshal([]byte(machine), &listed) != nil {
			return fmt.Sprintf("the listing did not come back: exit %d", code)
		}
		for _, entry := range listed.Sessions {
			if entry.Name == worker.name {
				return entry.Telemetry.Activity
			}
		}
		return worker.name + " is not listed"
	}

	var sends []string
	code, sent, _ := asks.ask(c, "send", owed.name, stoppedRoutingTask)
	sends = append(sends, fmt.Sprintf("task: exit %d, %s", code, firstLine(sent)))
	code, sent, _ = asks.ask(c, "send", free.name, stoppedRoutingNote, "--notify")
	sends = append(sends, fmt.Sprintf("notify: exit %d, %s", code, firstLine(sent)))
	var task reportView
	if !waitFor(c, 30*time.Second, func() bool {
		var read bool
		task, read = messageCarrying(owed, "stopped-routing:")
		_, noted := messageCarrying(free, "stopped-routing-note:")
		return read && noted
	}) {
		return unjudged(fmt.Sprintf("the workers did not read what was sent; sends: %v", sends))
	}

	var out []telemetryFinding
	about := func() []string {
		var kinds []string
		for _, message := range readMessages(lead) {
			if message.From == owed.name && slices.Contains(message.InReplyTo, task.ID) {
				kinds = append(kinds, message.Kind)
			}
		}
		return kinds
	}
	heard := waitFor(c, 15*time.Second, func() bool { return slices.Contains(about(), "stopped") })
	state, failure := awaitedState(task.ID)
	out = append(out, finding(obsStoppedToWaiter, heard && state == "stopped" && failure == "",
		"main read %v about the task; the awaited view gives it %q %s", about(), state, failure))

	// Each interruption was acted on once it left a receipt; whatever it sent
	// would be on its way by then, so a moment more is enough to see it.
	acted := waitFor(c, 15*time.Second, func() bool { return outcomes(free) > 0 && outcomes(lead) > 0 })
	time.Sleep(time.Second)
	freeHolds, idle := holds(free), activity(free)
	out = append(out, finding(obsStoppedToNobody, acted && len(freeHolds) == 0 && idle == "idle",
		"the notified worker published %d outcomes; main holds %v from it; it reads %q", outcomes(free), freeHolds, idle))
	leadHolds := holds(lead)
	out = append(out, finding(obsMainOwnEsc, acted && len(leadHolds) == 0,
		"main published %d outcomes and holds %v from itself", outcomes(lead), leadHolds))
	return out
}

// The control: the rule this scenario is for, undone — a stop nobody waits on
// goes to main, as it did before the owner's decision.
var mutantStoppedToMain = mutation{
	name:  "stopped-to-main",
	file:  "internal/cli/turn_reports.go",
	edits: []edit{{"\tif event.Failed && !event.Stopped && len(reports) == 0 {\n", "\tif (event.Failed || event.Stopped) && len(reports) == 0 {\n"}},
}

func TestAStoppedSentToMainFails(t *testing.T) {
	runInColumns(t, "stopped-routing-control-"+mutantStoppedToMain.name, func(t *testing.T, col column) {
		play := func(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
			return playStoppedRouting(t, c, iso, col)
		}
		if !col.offers(capabilitySelection) {
			if _, err := exec.LookPath("node"); err != nil {
				name := "stopped-routing-control-" + mutantStoppedToMain.name
				enterScenario(t, name)
				want := "the " + mutantStoppedToMain.name + " mutant breaks " + obsStoppedToNobody + "; " + obsMainOwnEsc + ", and nothing else"
				c := Start(t, Spec{Name: name, Harness: col.harness, Observations: []string{want}, Deadline: time.Minute})
				c.UnsupportedCapability(want, "node", "no node on PATH to run the plugin's module")
				return
			}
		}
		runFindingsControlOn(t, col.harness, "stopped-routing", play, mutantStoppedToMain, obsStoppedToNobody, obsMainOwnEsc)
	})
}
