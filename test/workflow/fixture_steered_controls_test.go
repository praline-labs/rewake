package workflow

import "testing"

// The controls of the steered case on the gate column. Each breaks one link of
// a request's path in the core — the command, the control protocol, main's
// wrapper and its letters, main's awaited list — and the observations resting
// on that link go with it.

// A request is sent to a run whatever its live set says.
var mutantSteersUnlive = mutation{
	name:  "steers-unlive",
	file:  "internal/cli/steer.go",
	edits: []edit{{"if !session.HasLive(string(adapter.ControlCapability)) {", "if false && !session.HasLive(string(adapter.ControlCapability)) {"}},
}

// Any session may steer another.
var mutantAnyRoleSteers = mutation{
	name:  "any-role-steers",
	file:  "internal/cli/steer.go",
	edits: []edit{{"if self.Role != role.Main.ID {", "if false && self.Role != role.Main.ID {"}},
}

// main's wrapper announces a compaction main asked for itself.
var mutantOwnCompactionAnnounced = mutation{
	name:  "own-compaction-announced",
	file:  "internal/host/session_notices.go",
	edits: []edit{{"if event.RequestedBy == self.Name {", "if false && event.RequestedBy == self.Name {"}},
}

// main's letter leaves out the session's count of compactions.
var mutantLetterUncounted = mutation{
	name:  "letter-uncounted",
	file:  "internal/host/compaction_letters.go",
	edits: []edit{{`text += fmt.Sprintf(" (compaction %d).", found.event.Sequence)`, `text += "."`}},
}

// main's wrapper waits for the worker's word however the worker went: a
// worker killed mid-compaction leaves main without a letter until the bound,
// long past the case.
var mutantOrphanUnlettered = mutation{
	name:  "orphan-unlettered",
	file:  "internal/host/compaction_letters.go",
	edits: []edit{{"\t\tcase gone:\n", "\t\tcase false && gone:\n"}},
}

// main's list of what it is owed puts every stop down to a person.
var mutantStopByAPerson = mutation{
	name:  "stop-by-a-person",
	file:  "internal/cli/inbox_awaited.go",
	edits: []edit{{`return detail("stopped")`, `return "stopped by a person"`}},
}

func TestMainNoticedOfItsOwnCompactionFails(t *testing.T) {
	runSteeredControl(t, mutantOwnCompactionAnnounced, obsCompactQuiet)
}

func TestALetterWithoutTheCountFails(t *testing.T) {
	runSteeredControl(t, mutantLetterUncounted, obsCompactLetter)
}

func TestALetterLostWithItsWorkerFails(t *testing.T) {
	runSteeredControl(t, mutantOrphanUnlettered, obsCompactOrphan)
}

func TestAnInterruptListedAsAPersonsStopFails(t *testing.T) {
	runSteeredControl(t, mutantStopByAPerson, obsInterruptBusy)
}

func TestASteerWithoutALiveControlFails(t *testing.T) {
	runSteeredControl(t, mutantSteersUnlive, obsNotAnswering)
}

func TestAWorkerThatSteersFails(t *testing.T) {
	runSteeredControl(t, mutantAnyRoleSteers, obsNotMain)
}

func runSteeredControl(t *testing.T, mutant mutation, breaks ...string) {
	t.Helper()
	runFindingsControlOn(t, fixtureColumn.harness, "fixture-steered", playSteered, mutant, breaks...)
}
