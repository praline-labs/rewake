package workflow

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"
)

// The mid-turn scenario's negative controls. Two of them are the ones the
// contract names, and one is a mutation of the product; each names the
// observation its breakage must take down.
//
// Why only one is a mutant. The scenario asks about a fork the *server* owns:
// rewake sends the same turn/start either way, and what differs is whether a
// turn was already running. Most ways to break that are ways to make the
// fixture behave differently — a turn that is not held open, a letter sent
// after it ends — and those are worlds, not defects. The one place the product
// decides anything here is whether it delivers while the recipient is working,
// and that is what `wait-for-idle` mutates.

// The product mutant. A comment in the delivery path says why the wrapper does
// not look at a status snapshot: "native start-or-steer chooses active/idle
// atomically; a status snapshot cannot safely decide that for a concurrent
// terminal". This does the thing that comment warns against — waits while the
// snapshot says working — which is exactly "rewake waited for the turn to end".
var mutantWaitForIdle = mutation{
	name: "wait-for-idle",
	file: "internal/harness/codex/gateway/reservation.go",
	edits: []edit{{
		"\t\tvalid := sameBinding(want, c.state.Binding) && c.owner.owns(c)\n" +
			"\t\tclosed := c.state.deliverySettled()\n",
		"\t\tvalid := sameBinding(want, c.state.Binding) && c.owner.owns(c)\n" +
			"\t\tclosed := c.state.deliverySettled()\n" +
			"\t\tif entry := c.observationThread(want.Thread); entry != nil && entry.snapshot.Activity != nil && *entry.snapshot.Activity == \"working\" {\n" +
			"\t\t\tclosed = false\n\t\t}\n",
	}},
}

// A letter that arrives after the turn has ended has not arrived mid-turn.
// Nothing is broken here — the delivery is perfectly good — which is the
// point: the observation has to be able to fail, or it says nothing.
func TestALetterAfterTheTurnIsNotMidTurn(t *testing.T) {
	runMidTurnControl(t, midTurnControls[0], mustBreak)
}

// The work that was running fails instead of answering. A case that passed on
// the delivery alone would call that a success.
func TestAFailedHeldOperationFails(t *testing.T) {
	runMidTurnControl(t, midTurnControls[1], mustBreak)
}

// And the product mutant: rewake holds the delivery while the recipient's
// status says working.
func TestWaitingForIdleBeforeDeliveringFails(t *testing.T) {
	runMidTurnControl(t, midTurnControls[2], mustBreak)
}

// midTurnControl is one control: how the world differs, and which observation
// its difference must take down.
type midTurnControl struct {
	name string
	// mutant is empty for a control expressed in the fixture.
	mutant   *mutation
	timing   midTurnTiming
	shim     []string
	expected string
}

var midTurnControls = []midTurnControl{
	{name: "late", timing: midTurnLate, expected: obsSteered},
	{name: "failed-operation", timing: midTurnLive, shim: []string{shimFailHeldTurn + "=1"}, expected: obsOriginalOutcome},
	{name: "wait-for-idle", mutant: &mutantWaitForIdle, timing: midTurnLive, expected: obsSteered},
}

// TestMidTurnControlsCrosswise checks each control's observation against the
// other controls' worlds. It is smaller than the batch-arrival matrix on
// purpose: two of the three controls name the same observation, so the cross
// is over the pairs whose observations differ.
//
// One pair is deliberately absent, and its absence is the finding: no pair can
// ask whether "the arrival created no second outcome" survives a world where
// nothing was steered. In such a world the question has no subject — there is
// no joined turn to count outcomes for — so the answer is "cannot judge"
// rather than "held", and a cell that must come out unjudgeable is not a
// control. It is written down in the scenario record instead.
func TestMidTurnControlsCrosswise(t *testing.T) {
	if os.Getenv(crossSwitch) == "" {
		t.Skipf("crosswise check skipped: set %s=1 to run every control against every other world", crossSwitch)
	}
	enterScenario(t, "mid-turn-crosswise")
	// Over observations rather than over controls: two of the three controls
	// name the same observation, and pairing control with control ran that
	// one twice under the same world, under two names for one question.
	for _, observation := range crossedObservations() {
		for _, other := range midTurnControls {
			if other.expected == observation {
				continue
			}
			t.Run(other.name+"-keeps-"+shortObservation(observation), func(t *testing.T) {
				runNamedMidTurnControl(t, "mid-turn-cross-"+shortObservation(observation)+"-under-"+other.name,
					other, observation, mustHold)
			})
		}
	}
}

// crossedObservations are the observations some control breaks, each once, in
// the order the controls declare them.
func crossedObservations() []string {
	var out []string
	for _, control := range midTurnControls {
		if !slices.Contains(out, control.expected) {
			out = append(out, control.expected)
		}
	}
	return out
}

// shortObservation is a name for a subtest: the observation's own words are a
// sentence, which reads badly as a test name and is repeated in the case name
// anyway.
func shortObservation(observation string) string {
	switch observation {
	case obsSteered:
		return "steer"
	case obsOriginalOutcome:
		return "original-outcome"
	}
	return "observation"
}

func runMidTurnControl(t *testing.T, control midTurnControl, want whatIsExpected) {
	t.Helper()
	enterScenario(t, "mid-turn-control-"+control.name)
	runNamedMidTurnControl(t, "mid-turn-control-"+control.name, control, control.expected, want)
}

// runNamedMidTurnControl runs the scenario's sessions in one control's world
// and records whether the named observation came out the way this pair
// requires.
func runNamedMidTurnControl(t *testing.T, name string, control midTurnControl, expected string, want whatIsExpected) {
	t.Helper()
	// The case first, then the mutant, for the reason given in buildMutant.
	c := Start(t, Spec{
		Name:         name,
		Harness:      "codex",
		Observations: []string{want.observation()},
		Deadline:     150 * time.Second,
	})
	binary := suite.binary
	if control.mutant != nil {
		built, err := buildMutant(c, *control.mutant)
		if err != nil {
			c.Contradicted(want.observation(), "the mutant could not be built: %v", err)
			return
		}
		binary = built
	}
	if !offersMidTurn(c, codexColumn) {
		return
	}
	iso := Isolate(t, c, binary)
	worker, sender := startMidTurnSessions(t, c, iso, codexColumn, control.timing, control.shim...)
	defer stopSession(t, c, worker)
	defer stopSession(t, c, sender)

	// Anchored on all three things a judgement here needs: the recipient read
	// the second letter, the turn it held open finished, and the sender read
	// an outcome. The second letter belongs in the anchor because in two of
	// these worlds it is delivered only after the held turn ends — judged
	// before that, every control reported that it could not tell, which is
	// the right answer to the wrong moment. The window is generous because
	// those worlds wait out the fixture's hold first.
	if !waitFor(c, midTurnWindow, func() bool {
		events, err := worker.turnEvents()
		if err != nil {
			return false
		}
		_, read := messageCarrying(worker, midTurnSecond)
		if !read {
			// A letter whose delivery was refused will never be read, and
			// waiting for it would spend the window on a settled question.
			sends, err := sender.sendRecords()
			if err != nil {
				return false
			}
			record, sent := sendOf(sends, "second")
			read = sent && record.Outcome == "refused"
		}
		return read && eventOf(events, "completed") != nil && outcomesRead(sender, worker) > 0
	}) {
		c.Contradicted(want.observation(), "the second letter was never read, or the held turn never finished and reported, so nothing could be judged")
		return
	}
	c.Note("watching " + expected + " under " + control.name)

	broke, why, err := midTurnControlOutcome(expected, worker, sender)
	switch {
	case err != nil:
		c.Contradicted(want.observation(), "could not judge %s under %s: %v", expected, control.name, err)
	case want == mustBreak && broke:
		c.Observed(want.observation(), why)
	case want == mustBreak:
		c.Contradicted(want.observation(), "%s held anyway: %s", expected, why)
	case broke:
		c.Contradicted(want.observation(), "%s broke under %s, whose breakage it does not cover: %s", expected, control.name, why)
	default:
		c.Observed(want.observation(), fmt.Sprintf("%s held under %s: %s", expected, control.name, why))
	}
}

// midTurnWindow bounds the wait for a control's world to finish. A world where
// nothing is steered waits out the fixture's own hold before its turn ends, so
// the ceiling has to clear that with room to spare.
const midTurnWindow = 90 * time.Second

// midTurnControlOutcome asks for the whole shape of each breakage, in three
// values: broken, not broken, or not judgeable. An empty or unreadable record
// is the third, never the second.
func midTurnControlOutcome(expected string, worker, sender *codexSession) (bool, string, error) {
	events, err := worker.turnEvents()
	if err != nil {
		return false, "", err
	}
	if len(events) == 0 {
		return false, "", errors.New("the recipient recorded no turn events, so nothing can be judged")
	}
	steered := eventOf(events, "steered")
	second, secondKnown := messageCarrying(worker, midTurnSecond)
	switch expected {
	case obsSteered:
		sends, err := sender.sendRecords()
		if err != nil {
			return false, "", err
		}
		record, sent := sendOf(sends, "second")
		switch {
		case secondKnown && (steered == nil || steered.Detail != second.ID):
			// It arrived, and not into an open turn.
			return true, fmt.Sprintf("the second letter %s reached the recipient outside any open turn; completions: %v",
				second.ID, details(allOf(events, "completed"))), nil
		case secondKnown:
			return false, "the second letter was steered into " + steered.Turn, nil
		case sent && record.Outcome == "refused":
			// It never arrived, and the sender's own rewake said why. That is
			// a delivery not accepted mid-turn as surely as a late one.
			return true, "the second letter never reached the recipient: " + record.Detail, nil
		case !sent:
			return false, "", errors.New("the sender recorded nothing about the second letter")
		}
		return false, "", errors.New("the sender's letter was accepted and the recipient has not read it yet")
	case obsOriginalOutcome:
		completions := allOf(events, "completed")
		if len(completions) == 0 {
			return false, "", errors.New("the recipient recorded no completion, so its outcome cannot be judged")
		}
		_, finished := reportOfKind(sender, worker, "finished")
		failure, failed := reportOfKind(sender, worker, "error")
		// Broken means the turn that was running did not answer: its own
		// record says it failed, and what reached the sender is an error
		// rather than the answer. Both halves, because either alone would also
		// be true of a run whose report simply had not arrived yet.
		if completions[0].Detail == "failed" && failed && !finished {
			return true, "the held turn ended failed and the sender was told " + failure.Kind, nil
		}
		return false, fmt.Sprintf("the held turn ended %s and the sender read a finished report: %v",
			completions[0].Detail, finished), nil
	case obsPending:
		open, closed := eventOf(events, "operation-open"), eventOf(events, "operation-closed")
		if open == nil || closed == nil {
			return false, "", errors.New("the recipient never recorded an operation it held open")
		}
		if steered == nil || open.At >= steered.At || steered.At >= closed.At {
			return true, "no delivery fell between the opening and the closing of the operation", nil
		}
		return false, "the delivery fell inside the open operation", nil
	}
	return false, "", errors.New("no such observation: " + expected)
}
