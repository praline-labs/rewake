package workflow

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestCodexSteered is `rewake compact` and `rewake interrupt` end to end on
// the Codex column: a main steers two workers, and each request travels the
// command, the control directory, the worker's wrapper serving it, the
// app-server's request, and back.
//
//   - calm works its task to the end and is idle: a compaction with a focus is
//     a wrong call and nothing is sent; one without is answered started once
//     its turn shows its compaction item, before the telemetry counts it; its
//     end reaches main as a letter with the tokens the context held before and
//     after and that count, and main gets no notice of it. An interrupt is
//     refused as no turn running.
//   - busy is held in its first turn: a compaction is refused as in a turn
//     before anything is sent — the server would abort the turn and compact
//     instead — and the turn goes on; an interrupt ends it, main reads stopped
//     naming itself, as its awaited list does, and busy's next notice carries
//     no line about it, because Codex puts the interrupt into the model's
//     history itself.
//
// And a worker asking is a wrong call.
//
// What it does not prove: that the real server answers as the fixture does.
// Its order and refusals were seen live on 0.155.1 on September 24, 2026 and
// are recorded in docs/research-protocol.md; the shapes are checked against
// the schema by the shape case.
func TestCodexSteered(t *testing.T) {
	binary := enterScenario(t, "codex-steered")
	c := Start(t, Spec{
		Name:         "codex-steered",
		Harness:      codexColumn.harness,
		Observations: codexSteeredObservations,
		Deadline:     120 * time.Second,
	})
	for _, finding := range playCodexSteered(t, c, Isolate(t, c, binary)) {
		if finding.held {
			c.Observed(finding.observation, finding.detail)
		} else {
			c.Contradicted(finding.observation, "%s", finding.detail)
		}
	}
}

const (
	obsFocusRefused    = "main compacts a Codex worker with a focus: a wrong call, and nothing is compacted"
	obsNoInterruptLine = "the interrupted worker's next notice carries no line about the interrupt"
	codexSteeredTask   = "codex-steered: work the long task"
	codexSteeredNote   = "codex-steered-note: go on"
	codexStoppedText   = "lead-codex interrupted this turn with rewake interrupt"
)

var codexSteeredObservations = []string{
	obsFocusRefused, obsCompactStarted, obsCompactLetter, obsCompactQuiet, obsCompactBusy, obsInterruptBusy, obsNoInterruptLine, obsInterruptIdle, obsNotMain,
}

func playCodexSteered(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	t.Helper()
	col := codexColumn.harness
	// Every session serves requests, which also keeps it up long enough for
	// the whole case.
	calmAsks := newRequests(iso, "calm")
	calm := startHarnessSession(t, c, iso, col, "calm", "--general", shimInboxJSON+"=1", shimCompactTakes+"="+compactTakes, calmAsks.env())
	defer stopSession(t, c, calm)
	busy := startHarnessSession(t, c, iso, col, "busy", "--general", shimInboxJSON+"=1", shimHoldTurn+"=1", newRequests(iso, "busy").env())
	defer stopSession(t, c, busy)
	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, col, "lead", "--main", shimInboxJSON+"=1", asks.env())
	defer stopSession(t, c, lead)

	finding := func(observation string, held bool, detail string, args ...any) telemetryFinding {
		return telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)}
	}
	sg := steering{c: c, asks: asks, lead: lead}
	steer, about, row, kinds := sg.steer, sg.about, sg.row, reportKinds

	var sends []string
	for _, worker := range []*codexSession{calm, busy} {
		code, out, _ := asks.ask(c, "send", worker.name, codexSteeredTask)
		sends = append(sends, fmt.Sprintf("to %s: exit %d, %s", worker.name, code, firstLine(out)))
	}
	var out []telemetryFinding
	tasks := map[*codexSession]string{}
	if !waitFor(c, 20*time.Second, func() bool {
		for _, worker := range []*codexSession{calm, busy} {
			message, read := messageCarrying(worker, "codex-steered:")
			if !read {
				return false
			}
			tasks[worker] = message.ID
		}
		return slices.Contains(kinds(about(calm, tasks[calm])), "finished")
	}) {
		for _, observation := range codexSteeredObservations[:len(codexSteeredObservations)-1] {
			out = append(out, telemetryFinding{observation: observation, detail: fmt.Sprintf("the workers did not take their tasks: sends %v; calm's reports %v", sends, kinds(about(calm, tasks[calm])))})
		}
		code, refused, _ := calmAsks.ask(c, "compact", busy.name)
		return append(out, finding(obsNotMain, code == 2, "exit %d, %s", code, firstLine(refused)))
	}

	// busy is in its held turn: a compaction is refused, and the turn is
	// still running afterwards — not aborted, as a compaction sent would have
	// done — with nothing reported about its task.
	code, view, said := steer("compact", busy.name)
	busyRow, failure := row(busy)
	early := kinds(about(busy, tasks[busy]))
	out = append(out, finding(obsCompactBusy,
		code == 1 && view.Outcome == "refused" && view.Reason == "in a turn" && busyRow.Activity == "working" && len(early) == 0 && failure == "",
		"%s; then busy reads %q %s, and main read %v about its task", said, busyRow.Activity, failure, early))

	code, view, said = steer("interrupt", busy.name)
	stopped := sg.stoppedAbout(busy, tasks[busy])
	_, awaited, _ := asks.ask(c, "inbox", "--awaited")
	out = append(out, finding(obsInterruptBusy,
		code == 0 && view.Outcome == "done" && strings.Contains(stopped.Text, codexStoppedText) &&
			strings.Contains(awaited, " · stopped: "+codexStoppedText+"\n"),
		"%s; main read %v about the task, the stopped saying %q; its awaited list: %q", said, kinds(about(busy, tasks[busy])), firstLine(stopped.Text), awaited))

	code, sent, _ := asks.ask(c, "send", busy.name, codexSteeredNote, "--notify")
	var deliveries []groupDelivery
	var err error
	waitFor(c, 10*time.Second, func() bool {
		deliveries, err = busy.groupDeliveries()
		return err == nil && len(deliveries) >= 2
	})
	var notices []string
	for _, delivery := range deliveries {
		notices = append(notices, fmt.Sprintf("%s: %q", delivery.Turn, delivery.Notice))
	}
	out = append(out, finding(obsNoInterruptLine,
		len(deliveries) == 2 && deliveries[1].Notice != "" && !strings.Contains(deliveries[1].Notice, "interrupt"),
		"busy's notices: %s; the note: exit %d, %s", strings.Join(notices, "; "), code, firstLine(sent)))

	code, view, said = steer("interrupt", calm.name)
	out = append(out, finding(obsInterruptIdle, code == 1 && view.Outcome == "refused" && view.Reason == "no turn running", "%s", said))

	// A focus first: had it been compacted, the plain compaction after it
	// would be the second, and its letter would say so.
	focusCode, _, focusSaid := steer("compact", calm.name, steeredFocus)
	out = append(out, sg.compactIdle(calm, workTokens, compactedTokens)...)
	calmRow, failure := row(calm)
	out = append(out, finding(obsFocusRefused,
		focusCode == 2 && strings.Contains(focusSaid, "focus") && calmRow.Compactions != nil && *calmRow.Compactions == 1,
		"%s; the telemetry then counts %s compactions %s", focusSaid, show(calmRow.Compactions), failure))

	code, refused, _ := calmAsks.ask(c, "compact", busy.name)
	out = append(out, finding(obsNotMain, code == 2, "exit %d, %s", code, firstLine(refused)))
	return out
}
