package workflow

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestClaudeSteered is `rewake compact` and `rewake interrupt` end to end on
// the Claude Code column: a main steers three workers through the control
// directory, and each request travels the product's whole path — the command,
// the directory, the module polling it under node, the session carrying it
// out, and back.
//
//   - calm works its task to the end and is idle: a compaction is done, with
//     the host's token counts, and rewake's telemetry counts it by the hooks a
//     compaction runs; the answer carries that count, and main, which asked,
//     gets no notice of the compaction besides. An interrupt is refused as no
//     turn running.
//   - busy is held in its first turn: a compaction is refused as in a turn and
//     the turn goes on; an interrupt ends it, main reads stopped naming itself
//     and its list of what it is owed says the same, and busy's next notice
//     says who interrupted it — once.
//   - bare runs without the module: every request is not answering.
//
// And a worker asking is a wrong call.
//
// What it does not prove: that the real harness carries these calls out the
// way the fixture does. Its forms, refusals and effects were seen live by the
// review of September 24, 2026 and are recorded in
// docs/research-claude-actions.md; the fixture plays them, strictly.
func TestClaudeSteered(t *testing.T) {
	binary := enterScenario(t, "claude-steered")
	c := Start(t, Spec{
		Name:         "claude-steered",
		Harness:      claudeColumn.harness,
		Observations: steeredObservations,
		Deadline:     120 * time.Second,
	})
	if _, err := exec.LookPath("node"); err != nil {
		for _, observation := range steeredObservations {
			c.UnsupportedCapability(observation, "node", "no node on PATH to run the plugin's module")
		}
		return
	}
	for _, finding := range playSteered(t, c, Isolate(t, c, binary)) {
		if finding.held {
			c.Observed(finding.observation, finding.detail)
		} else {
			c.Contradicted(finding.observation, "%s", finding.detail)
		}
	}
}

const (
	obsCompactIdle     = "main compacts an idle worker: done with the token counts, and the telemetry counts one compaction"
	obsCompactBusy     = "main compacts a worker in a turn: refused as in a turn, and the turn goes on unreported"
	obsInterruptBusy   = "main interrupts a worker in a turn: done, and main reads stopped saying main interrupted it, as its awaited list does"
	obsCompactQuiet    = "main's own compaction: its answer carries the session's compaction count, and main gets no compaction notice of it"
	obsInterruptLine   = "the interrupted worker's next notice says main interrupted it, and the one after does not"
	obsInterruptIdle   = "main interrupts an idle worker: refused as no turn running"
	obsNotAnswering    = "a request to a worker without the module is refused as not answering"
	obsNotMain         = "a worker that is not main asking for a compaction is a wrong call"
	steeredTask        = "claude-steered: work the long task"
	steeredNote        = "claude-steered-note: go on"
	steeredFocus       = "keep the plan"
	steeredStoppedText = "lead-claude interrupted this turn with rewake interrupt"
	steeredNoticeLine  = "lead-claude interrupted your previous turn with rewake interrupt."
)

var steeredObservations = []string{
	obsCompactIdle, obsCompactQuiet, obsCompactBusy, obsInterruptBusy, obsInterruptLine, obsInterruptIdle, obsNotAnswering, obsNotMain,
}

// steeredView is what `rewake compact` and `rewake interrupt` print under
// --json.
type steeredView struct {
	Outcome      string  `json:"outcome"`
	Reason       string  `json:"reason"`
	Detail       string  `json:"detail"`
	TokensBefore *int64  `json:"tokensBefore"`
	TokensAfter  *int64  `json:"tokensAfter"`
	Compaction   *uint64 `json:"compaction"`
}

type steeredRow struct {
	Activity    string  `json:"activity"`
	Compactions *uint64 `json:"completedCompactions"`
}

func playSteered(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		var out []telemetryFinding
		for _, observation := range steeredObservations {
			out = append(out, telemetryFinding{observation: observation, detail: "no node to run the plugin's module"})
		}
		return out
	}
	runs := shimNode + "=" + node
	calmAsks := newRequests(iso, "calm")
	calm := startHarnessSession(t, c, iso, claudeColumn.harness, "calm", "--general", shimInboxJSON+"=1", runs, calmAsks.env())
	defer stopSession(t, c, calm)
	busy := startHarnessSession(t, c, iso, claudeColumn.harness, "busy", "--general", shimInboxJSON+"=1", runs, shimHoldFirstTurn+"=1")
	defer stopSession(t, c, busy)
	bare := startHarnessSession(t, c, iso, claudeColumn.harness, "bare", "--general", shimInboxJSON+"=1", runs, shimNoFunctionHooks+"=1")
	defer stopSession(t, c, bare)
	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, claudeColumn.harness, "lead", "--main", shimInboxJSON+"=1", asks.env())
	defer stopSession(t, c, lead)

	finding := func(observation string, held bool, detail string, args ...any) telemetryFinding {
		return telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)}
	}
	// steer runs one command as main and reads what it printed.
	steer := func(args ...string) (int, steeredView, string) {
		code, out, ok := asks.ask(c, append(args, "--json")...)
		var view steeredView
		if !ok || json.Unmarshal([]byte(out), &view) != nil {
			return code, view, fmt.Sprintf("exit %d, %q", code, firstLine(out))
		}
		return code, view, fmt.Sprintf("exit %d, %s %s: %s", code, view.Outcome, view.Reason, view.Detail)
	}
	// The messages main read from a worker about its task.
	about := func(worker *codexSession, task string) []reportView {
		var found []reportView
		for _, message := range readMessages(lead) {
			if message.From == worker.name && slices.Contains(message.InReplyTo, task) {
				found = append(found, message)
			}
		}
		return found
	}
	kinds := func(messages []reportView) []string {
		var out []string
		for _, message := range messages {
			out = append(out, message.Kind)
		}
		return out
	}
	row := func(worker *codexSession) (steeredRow, string) {
		code, machine, ok := asks.ask(c, "list", "--json")
		var listed struct {
			Sessions []struct {
				Name      string     `json:"name"`
				Telemetry steeredRow `json:"telemetry"`
			} `json:"sessions"`
		}
		if !ok || code != 0 || json.Unmarshal([]byte(machine), &listed) != nil {
			return steeredRow{}, fmt.Sprintf("the listing did not come back: exit %d, %q", code, firstLine(machine))
		}
		for _, entry := range listed.Sessions {
			if entry.Name == worker.name {
				return entry.Telemetry, ""
			}
		}
		return steeredRow{}, worker.name + " is not listed"
	}

	var sends []string
	for _, worker := range []*codexSession{calm, busy} {
		code, out, _ := asks.ask(c, "send", worker.name, steeredTask)
		sends = append(sends, fmt.Sprintf("to %s: exit %d, %s", worker.name, code, firstLine(out)))
	}
	var out []telemetryFinding

	// While the two work: nothing serves bare's directory, so the request
	// waits out the pickup limit and is withdrawn.
	code, view, said := steer("compact", bare.name)
	out = append(out, finding(obsNotAnswering, code == 1 && view.Outcome == "refused" && view.Reason == "not answering", "%s", said))

	tasks := map[*codexSession]string{}
	if !waitFor(c, 20*time.Second, func() bool {
		for _, worker := range []*codexSession{calm, busy} {
			message, read := messageCarrying(worker, "claude-steered:")
			if !read {
				return false
			}
			tasks[worker] = message.ID
		}
		return slices.Contains(kinds(about(calm, tasks[calm])), "finished")
	}) {
		for _, observation := range steeredObservations[:len(steeredObservations)-2] {
			out = append(out, telemetryFinding{observation: observation, detail: fmt.Sprintf("the workers did not take their tasks: sends %v; calm's reports %v", sends, kinds(about(calm, tasks[calm])))})
		}
		code, refused, _ := calmAsks.ask(c, "compact", bare.name)
		return append(out, finding(obsNotMain, code == 2, "exit %d, %s", code, firstLine(refused)))
	}

	// busy is in its held turn: a compaction is refused, and the turn is
	// still running afterwards, with nothing reported about its task.
	code, view, said = steer("compact", busy.name)
	busyRow, failure := row(busy)
	early := kinds(about(busy, tasks[busy]))
	out = append(out, finding(obsCompactBusy,
		code == 1 && view.Outcome == "refused" && view.Reason == "in a turn" && busyRow.Activity == "working" && len(early) == 0 && failure == "",
		"%s; then busy reads %q %s, and main read %v about its task", said, busyRow.Activity, failure, early))

	code, view, said = steer("interrupt", busy.name)
	var stopped reportView
	waitFor(c, 10*time.Second, func() bool {
		for _, message := range about(busy, tasks[busy]) {
			if message.Kind == "stopped" {
				stopped = message
				return true
			}
		}
		return false
	})
	// The task is still owed after a stop, and main's list of what it is owed
	// says who stopped it.
	_, awaited, _ := asks.ask(c, "inbox", "--awaited")
	out = append(out, finding(obsInterruptBusy,
		code == 0 && view.Outcome == "done" && strings.Contains(stopped.Text, steeredStoppedText) &&
			strings.Contains(awaited, " · stopped: "+steeredStoppedText+"\n"),
		"%s; main read %v about the task, the stopped saying %q; its awaited list: %q", said, kinds(about(busy, tasks[busy])), firstLine(stopped.Text), awaited))

	// Two notes, the second once the first one's turn is over, so that each
	// is a notice of its own.
	code, sent, _ := asks.ask(c, "send", busy.name, steeredNote+" one", "--notify")
	sends = append(sends, fmt.Sprintf("note one: exit %d, %s", code, firstLine(sent)))
	waitFor(c, 10*time.Second, func() bool { return strings.Contains(busy.acceptedTurns(), "delivery-2 ") })
	code, sent, _ = asks.ask(c, "send", busy.name, steeredNote+" two", "--notify")
	sends = append(sends, fmt.Sprintf("note two: exit %d, %s", code, firstLine(sent)))
	var deliveries []groupDelivery
	waitFor(c, 10*time.Second, func() bool {
		deliveries, err = busy.groupDeliveries()
		return err == nil && len(deliveries) >= 3
	})
	var notices []string
	for _, delivery := range deliveries {
		notices = append(notices, fmt.Sprintf("%s: %q", delivery.Turn, delivery.Notice))
	}
	out = append(out, finding(obsInterruptLine,
		len(deliveries) == 3 && !strings.Contains(deliveries[0].Notice, steeredNoticeLine) &&
			strings.HasSuffix(deliveries[1].Notice, "\n"+steeredNoticeLine) && !strings.Contains(deliveries[2].Notice, steeredNoticeLine),
		"busy's notices: %s; sends: %v", strings.Join(notices, "; "), sends))

	code, view, said = steer("interrupt", calm.name)
	out = append(out, finding(obsInterruptIdle, code == 1 && view.Outcome == "refused" && view.Reason == "no turn running", "%s", said))

	code, view, said = steer("compact", calm.name, steeredFocus)
	var calmRow steeredRow
	waitFor(c, 5*time.Second, func() bool {
		calmRow, failure = row(calm)
		return calmRow.Compactions != nil && *calmRow.Compactions == 1
	})
	counted := calmRow.Compactions != nil && *calmRow.Compactions == 1
	out = append(out, finding(obsCompactIdle,
		code == 0 && view.Outcome == "done" && view.TokensBefore != nil && *view.TokensBefore == 120000 && view.TokensAfter != nil && *view.TokensAfter == 9000 && counted,
		"%s, tokens %s before and %s after; the telemetry counts %s compactions %s", said, show(view.TokensBefore), show(view.TokensAfter), show(calmRow.Compactions), failure))

	// main's wrapper looks for compactions once a second, and the telemetry
	// case sees its notice within five: nothing in five means none was sent.
	// No later event can stand in for the wait — the absence is the finding.
	const notice = "Rewake: context compacted (compaction 1)."
	noticed := waitFor(c, 5*time.Second, func() bool { return strings.Contains(lead.mailboxRead(), notice) })
	out = append(out, finding(obsCompactQuiet,
		code == 0 && view.Compaction != nil && *view.Compaction == 1 && !noticed,
		"the answer's compaction %s; main read %q: %v", show(view.Compaction), notice, noticed))

	code, refused, _ := calmAsks.ask(c, "compact", bare.name)
	out = append(out, finding(obsNotMain, code == 2, "exit %d, %s", code, firstLine(refused)))
	return out
}
