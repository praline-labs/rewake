package workflow

import "testing"

// The controls of the Codex column. Each breaks one link of a request's path
// in the product, and the observations resting on that link go with it.

// The wrapper sends a compaction whatever runs. The server then aborts the
// held turn in its place, so the interrupt after it finds no turn either.
var mutantBusyUnchecked = mutation{
	name:  "busy-unchecked",
	file:  "internal/harness/codex/gateway/steer.go",
	edits: []edit{{`if why := c.busyWith(binding.Thread); why != "" {`, `if why := c.busyWith(binding.Thread); false && why != "" {`}},
}

// The compaction is counted without the request and the asker: main gets a
// notice and the command finds no count.
var mutantCodexAskerUntold = mutation{
	name:  "codex-asker-untold",
	file:  "internal/harness/codex/gateway/telemetry_fields.go",
	edits: []edit{{"event.request, event.by = a.request, a.by", "_ = a"}},
}

// The wrapper interrupts without keeping who asked.
var mutantCodexInterrupterUnnamed = mutation{
	name:  "codex-interrupter-unnamed",
	file:  "internal/harness/codex/gateway/steer.go",
	edits: []edit{{`c.interrupted = steered{id: binding.Thread + "/" + turn, by: by}`, `c.interrupted = steered{}`}},
}

// The wrapper answers an interrupt of an idle session as done.
var mutantCodexIdleInterruptDone = mutation{
	name: "codex-idle-interrupt-done",
	file: "internal/harness/codex/gateway/steer.go",
	edits: []edit{{
		"	if !d.running {\n		c.mu.Unlock()\n		return control.Answer{Outcome: control.Refused, Reason: control.NoTurn}",
		"	if !d.running {\n		c.mu.Unlock()\n		return control.Answer{Outcome: control.Done}",
	}},
}

// The command lets a focus through to the Codex wrapper, which cannot carry
// it out: the call is no longer refused as wrong.
var mutantFocusTaken = mutation{
	name:  "focus-taken",
	file:  "internal/harness/codex/codex.go",
	edits: []edit{{"func (codexHarness) CompactFocus() bool { return false }", "func (codexHarness) CompactFocus() bool { return true }"}},
}

func TestACompactionSentMidTurnFails(t *testing.T) {
	runCodexSteeredControl(t, mutantBusyUnchecked, obsCompactBusy, obsInterruptBusy)
}

func TestACodexCompactionNotTiedToItsAskerFails(t *testing.T) {
	runCodexSteeredControl(t, mutantCodexAskerUntold, obsCompactQuiet)
}

func TestACodexInterruptThatDoesNotNameMainFails(t *testing.T) {
	runCodexSteeredControl(t, mutantCodexInterrupterUnnamed, obsInterruptBusy)
}

func TestACodexIdleInterruptAnsweredDoneFails(t *testing.T) {
	runCodexSteeredControl(t, mutantCodexIdleInterruptDone, obsInterruptIdle)
}

func TestAFocusTakenOnCodexFails(t *testing.T) {
	runCodexSteeredControl(t, mutantFocusTaken, obsFocusRefused)
}

func runCodexSteeredControl(t *testing.T, mutant mutation, breaks ...string) {
	t.Helper()
	runFindingsControlOn(t, codexColumn.harness, "codex-steered", playCodexSteered, mutant, breaks...)
}
