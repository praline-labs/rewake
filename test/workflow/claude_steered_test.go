package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestClaudeSteered is `rewake compact` and `rewake interrupt` end to end on
// the Claude Code column: a main steers three workers through the control
// directory, and each request travels the product's whole path — the command,
// the directory, the module polling it under node, the session carrying it
// out, and back.
//
//   - calm works its task to the end and is idle: a compaction is answered
//     started while it still runs, before rewake's telemetry counts it by the
//     hooks a compaction runs; its end reaches main as a letter with the
//     host's token counts and that count, and main, which asked, gets no
//     notice of the compaction besides. An interrupt is refused as no turn
//     running. Then calm is killed in the middle of a second compaction, and
//     main still gets its letter, from its own wrapper's record.
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
	obsCompactStarted  = "main compacts an idle worker: the command answers started before the compaction ends, which the telemetry has not counted yet"
	obsCompactLetter   = "the compaction's end reaches main as a notify from the worker, with the token counts and the compaction's number"
	obsCompactBusy     = "main compacts a worker in a turn: refused as in a turn, and the turn goes on unreported"
	obsInterruptBusy   = "main interrupts a worker in a turn: done, and main reads stopped saying main interrupted it, as its awaited list does"
	obsCompactQuiet    = "main's own compaction: the telemetry counts it, and main gets no compaction notice of it"
	obsInterruptLine   = "the interrupted worker's next notice says main interrupted it, and the one after does not"
	obsInterruptIdle   = "main interrupts an idle worker: refused as no turn running"
	obsCompactOrphan   = "a worker killed in the middle of a compaction main asked for: main still gets one letter of it"
	obsNotAnswering    = "a request to a worker without the module is refused as not answering"
	obsNotMain         = "a worker that is not main asking for a compaction is a wrong call"
	steeredTask        = "claude-steered: work the long task"
	steeredNote        = "claude-steered-note: go on"
	steeredFocus       = "keep the plan"
	steeredStoppedText = "lead-claude interrupted this turn with rewake interrupt"
	steeredNoticeLine  = "lead-claude interrupted your previous turn with rewake interrupt."
)

var steeredObservations = []string{
	obsCompactStarted, obsCompactLetter, obsCompactQuiet, obsCompactBusy, obsInterruptBusy, obsInterruptLine, obsInterruptIdle, obsCompactOrphan,
	obsNotAnswering, obsNotMain,
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
	calm := startHarnessSession(t, c, iso, claudeColumn.harness, "calm", "--general", shimInboxJSON+"=1", runs, shimCompactTakes+"="+compactTakes, calmAsks.env())
	defer stopSession(t, c, calm)
	// busy and bare are asked nothing, but the case outlives the ordinary
	// ceiling.
	busy := startHarnessSession(t, c, iso, claudeColumn.harness, "busy", "--general", shimInboxJSON+"=1", runs, shimHoldFirstTurn+"=1", staysUp(iso, "busy"))
	defer stopSession(t, c, busy)
	bare := startHarnessSession(t, c, iso, claudeColumn.harness, "bare", "--general", shimInboxJSON+"=1", runs, shimNoFunctionHooks+"=1", staysUp(iso, "bare"))
	defer stopSession(t, c, bare)
	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, claudeColumn.harness, "lead", "--main", shimInboxJSON+"=1", asks.env())
	defer stopSession(t, c, lead)

	finding := func(observation string, held bool, detail string, args ...any) telemetryFinding {
		return telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)}
	}
	sg := steering{c: c, asks: asks, lead: lead}
	steer, about, row, kinds := sg.steer, sg.about, sg.row, reportKinds

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
		return append(out, finding(obsNotMain, code == 2 && strings.Contains(refused, notMainRefusal), "exit %d, %s", code, firstLine(refused)))
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
	stopped := sg.stoppedAbout(busy, tasks[busy])
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

	// With a focus, which only this column takes: the fixture's compaction
	// runs it, as the host does.
	out = append(out, sg.compactIdle(calm, 120000, 9000, steeredFocus)...)

	code, refused, _ := calmAsks.ask(c, "compact", bare.name)
	out = append(out, finding(obsNotMain, code == 2 && strings.Contains(refused, notMainRefusal), "exit %d, %s", code, firstLine(refused)))

	// calm's harness is killed while its second compaction runs: nothing
	// records the end but, at most, the module's report that the host's call
	// failed or the count of a compaction that just ended, and the letter comes
	// from main's own wrapper either way. A worker stopped the ordinary way may
	// finish the compaction first, so it is killed. A letter without an outcome
	// goes 3 seconds after the departure is seen.
	lettersOf := func() []string {
		var letters []string
		for _, message := range readMessages(lead) {
			if message.From == calm.name && (strings.HasPrefix(message.Text, "Rewake: the compaction of "+calm.name+" you asked for") ||
				strings.HasPrefix(message.Text, "Rewake: compacted "+calm.name)) {
				letters = append(letters, message.Kind+" "+message.Text)
			}
		}
		return letters
	}
	earlier := len(lettersOf())
	code, view, said = steer("compact", calm.name)
	killHarness(c, iso, calm)
	var letters []string
	waitFor(c, 15*time.Second, func() bool {
		letters = lettersOf()
		letters = letters[min(earlier, len(letters)):]
		return len(letters) > 0
	})
	out = append(out, finding(obsCompactOrphan,
		code == 0 && view.Outcome == "started" && len(letters) == 1 && strings.HasPrefix(letters[0], "notify "),
		"%s; then main read %q", said, letters))
	return out
}

// killHarness kills a session's harness outright, as a crash does, and waits
// for its wrapper to end the session: nothing of the harness writes anything
// afterwards, while the wrapper still cleans up after it.
func killHarness(c *Case, iso *Isolation, session *codexSession) {
	var record struct {
		HarnessPID int `json:"harnessPid"`
	}
	raw, err := os.ReadFile(filepath.Join(iso.StateDir, "rooms", "default", "sessions", session.name+".json"))
	if err != nil || json.Unmarshal(raw, &record) != nil || record.HarnessPID <= 0 {
		c.Note("no harness to kill for " + session.name)
		return
	}
	c.mu.Lock()
	session.process.endedByCase = true
	c.mu.Unlock()
	_ = syscall.Kill(record.HarnessPID, syscall.SIGKILL)
	waitFor(c, 10*time.Second, func() bool { return session.process.finished() && !session.process.groupAlive() })
}
