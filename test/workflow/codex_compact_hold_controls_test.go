package workflow

import "testing"

// The controls of compact-hold, each the product as it was before one part of
// the fix, or with that part undone.

// The mark stops holding at its start bound although its compaction is seen
// running, as every mark did at its one bound of 80 seconds.
var mutantHoldEndsAtStart = mutation{
	name:  "hold-ends-at-start",
	file:  "internal/harness/codex/gateway/activity.go",
	edits: []edit{{`case marker.turn != "" && now.Sub(marker.asked) >= run:`, `case marker.turn != "" && now.Sub(marker.asked) >= start:`}},
}

// The server's refusal of input for a compaction running fails the delivery
// for good, as it did.
var mutantCompactionRefusalFinal = mutation{
	name:  "compaction-refusal-final",
	file:  "internal/harness/codex/server_delivery.go",
	edits: []edit{{"	if errors.Is(err, gateway.ErrCompacting) {\n		// The server", "	if false && errors.Is(err, gateway.ErrCompacting) {\n		// The server"}},
}

// The end of a compaction that outlived the wrapper's wait is not recorded:
// main's letter comes from the count alone, without the tokens.
var mutantLateEndUnrecorded = mutation{
	name:  "late-end-unrecorded",
	file:  "internal/harness/codex/gateway/event_stream.go",
	edits: []edit{{"			c.recordLateEnds()\n", ""}},
}

// Main's wrapper takes a compaction still running for its outcome and writes
// the letter at once.
var mutantRunningTakenForAnOutcome = mutation{
	name:  "running-taken-for-an-outcome",
	file:  "internal/wrap/compaction_letters.go",
	edits: []edit{{"if found.outcome != nil && found.outcome.Outcome == control.Started {", "if false && found.outcome != nil && found.outcome.Outcome == control.Started {"}},
}

func TestAHoldEndingAtItsStartBoundFails(t *testing.T) {
	runCompactHoldControl(t, mutantHoldEndsAtStart, obsHeldPastStart)
}

func TestACompactionRefusalThatFailsTheTaskFails(t *testing.T) {
	runCompactHoldControl(t, mutantCompactionRefusalFinal, obsRefusedWaits)
}

func TestALateEndLeftUnrecordedFails(t *testing.T) {
	runCompactHoldControl(t, mutantLateEndUnrecorded, obsLateLetter)
}

func TestARunningCompactionTakenForAnOutcomeFails(t *testing.T) {
	runCompactHoldControl(t, mutantRunningTakenForAnOutcome, obsLateLetter)
}

func runCompactHoldControl(t *testing.T, mutant mutation, breaks ...string) {
	t.Helper()
	runFindingsControlOn(t, codexColumn.harness, "codex-compact-hold", playCompactHold, mutant, breaks...)
}
