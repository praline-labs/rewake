package workflow

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestCodexCompactHold is a task sent to a Codex worker right after main
// asked for its compaction, when the compaction runs long: the order a main
// uses to hand a compacted worker its next task. The server refuses input
// while a compaction runs, rather than queue it.
//
//   - slow compacts past the start bound of its mark (suiteCompactionStart)
//     but within its running bound: the mark, tied to the compaction's turn,
//     holds the task until the turn ends, and the task goes then, the server
//     never asked to take it meanwhile.
//   - slower compacts past the running bound too (suiteCompactionRun): the
//     hold ends there and the task goes, the server refuses it, and it waits
//     and goes again once the compaction has ended — refused, not failed. The
//     wrapper's wait for the end has ended by then, and main's letter comes
//     from the end all the same, with the tokens, not as a failure.
//
// Seen live on 0.155.1 on September 26, 2026: a compaction of a long
// conversation outlasted the 80 seconds the mark then held for, the task sent
// after it was refused with ActiveTurnNotSteerable and failed for good, and
// main was told the compaction had failed (docs/research-codex.md).
//
// What it does not prove: how long a real compaction takes, or that the real
// server refuses with these words on every version; the refusal's text is
// 0.155.1's.
func TestCodexCompactHold(t *testing.T) {
	binary := enterScenario(t, "codex-compact-hold")
	c := Start(t, Spec{
		Name:         "codex-compact-hold",
		Harness:      codexColumn.harness,
		Observations: compactHoldObservations,
		Deadline:     120 * time.Second,
	})
	for _, finding := range playCompactHold(t, c, Isolate(t, c, binary)) {
		if finding.held {
			c.Observed(finding.observation, finding.detail)
		} else {
			c.Contradicted(finding.observation, "%s", finding.detail)
		}
	}
}

const (
	obsHeldPastStart = "a task sent while a compaction runs past its mark's start bound goes once it ends, never refused"
	obsRefusedWaits  = "a task the server refuses for a compaction still running past its mark's bound waits and goes once it ends"
	obsLateLetter    = "a compaction ending after the wrapper's wait reaches main as a letter with its tokens, not as a failure"
	compactHoldWarm  = "compact-hold-warm: work a turn"
	compactHoldNext  = "compact-hold-next: the task after the compaction"
	// slowTakes is past suiteCompactionStart and within suiteCompactionRun,
	// slowerTakes past suiteCompactionRun, each with room for a loaded machine.
	slowTakes   = "4s"
	slowerTakes = "10s"
)

var compactHoldObservations = []string{obsHeldPastStart, obsRefusedWaits, obsLateLetter}

func playCompactHold(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	t.Helper()
	col := codexColumn.harness
	slow := startHarnessSession(t, c, iso, col, "slow", "--general", shimInboxJSON+"=1", shimCompactTakes+"="+slowTakes, staysUp(iso, "slow"))
	defer stopSession(t, c, slow)
	slower := startHarnessSession(t, c, iso, col, "slower", "--general", shimInboxJSON+"=1", shimCompactTakes+"="+slowerTakes, staysUp(iso, "slower"))
	defer stopSession(t, c, slower)
	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, col, "lead", "--main", shimInboxJSON+"=1", asks.env())
	defer stopSession(t, c, lead)
	sg := steering{c: c, asks: asks, lead: lead}
	workers := []*codexSession{slow, slower}

	// A conversation that has run no turn is not compacted: each works one
	// first, to its end.
	warm := map[*codexSession]string{}
	for _, worker := range workers {
		_, sent, _ := asks.ask(c, "send", worker.name, compactHoldWarm)
		warm[worker] = printedID(sent)
	}
	if !waitFor(c, 20*time.Second, func() bool {
		for _, worker := range workers {
			if !slices.Contains(reportKinds(sg.about(worker, warm[worker])), "finished") {
				return false
			}
		}
		return true
	}) {
		return unjudgedAll(compactHoldObservations, "the workers did not finish their first tasks: %v", warm)
	}

	next := map[*codexSession]string{}
	var asked []string
	for _, worker := range workers {
		_, _, said := sg.steer("compact", worker.name)
		code, sent, _ := asks.ask(c, "send", worker.name, compactHoldNext)
		next[worker] = printedID(sent)
		asked = append(asked, fmt.Sprintf("%s: compact %s; send exit %d, %q", worker.name, said, code, firstLine(sent)))
	}

	var out []telemetryFinding
	for _, worker := range workers {
		took := waitFor(c, 30*time.Second, func() bool {
			_, read := messageCarrying(worker, "compact-hold-next:")
			return read
		})
		events, err := worker.turnEvents()
		refused := allOf(events, "refused")
		ended := eventOf(events, "compaction-ended")
		state := statusState(iso, worker, next[worker])
		detail := fmt.Sprintf("%s; %s took the task: %v, its status %q; the fixture refused %d notices, the compaction's end at %v; its log %+v %v",
			strings.Join(asked, "; "), worker.name, took, state, len(refused), ended, events, err)
		if worker == slow {
			out = append(out, judged(obsHeldPastStart, took && state != "failed" && err == nil && ended != nil && len(refused) == 0, "%s", detail))
			continue
		}
		after := ended != nil && len(refused) > 0 && refused[0].At < ended.At
		out = append(out, judged(obsRefusedWaits, took && state != "failed" && err == nil && after, "%s", detail))
	}

	want := fmt.Sprintf("Rewake: compacted %s: %d tokens before, %d after (compaction 1).", slower.name, workTokens, compactedTokens)
	var letters []string
	waitFor(c, 15*time.Second, func() bool {
		letters = nil
		for _, message := range readMessages(lead) {
			if message.From == slower.name && (strings.HasPrefix(message.Text, "Rewake: compacted") || strings.Contains(message.Text, "compaction of")) {
				letters = append(letters, message.Text)
			}
		}
		return slices.Contains(letters, want)
	})
	return append(out, judged(obsLateLetter, len(letters) == 1 && letters[0] == want, "main read from %s: %q, want only %q", slower.name, letters, want))
}
