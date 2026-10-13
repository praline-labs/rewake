package workflow

import "testing"

// The controls of fixture-decide-sandboxed, each the product with one part of
// the verified path from a sandboxed recipient undone.

// A wrapper out of reach is taken for the person: the sandboxed shell's
// decide records an unverified decision, which the tool call then finds made.
var mutantUnreachableAsPerson = mutation{
	name: "decide-unreachable-as-person",
	file: "internal/cli/decide.go",
	edits: []edit{{
		"if errors.Is(err, authority.ErrUnreachable) {",
		"if errors.Is(err, authority.ErrUnreachable) {\n\t\t\t\task.By = mail.DecidedBy{Kind: mail.ByPerson}\n\t\t\t\tanswer, err = mail.Decide(context.Background(), dir, ask)\n\t\t\t}\n\t\t\tif false {",
	}},
}

// The tool's decide never asks the wrapper: its child is classified for the
// person, and being below a session it is refused.
var mutantToolDecidesAsPerson = mutation{
	name:  "decide-tool-as-person",
	file:  "internal/cli/decide.go",
	edits: []edit{{"if run, ok := authority.RunOf(subject.RecipientRun); ok && runsBelow(run) {", "if run, ok := authority.RunOf(subject.RecipientRun); ok && runsBelow(run) && ctx.scope == nil {"}},
}

// The wrapper writes what it is asked as the person's: recorded, unverified.
var mutantWrapperDecidesUnverified = mutation{
	name:  "decide-wrapper-unverified",
	file:  "internal/host/decide.go",
	edits: []edit{{"ask.By = mail.DecidedBy{Kind: mail.ByRecipient, Name: name, Run: epoch, Verified: true}", "ask.By = mail.DecidedBy{Kind: mail.ByPerson}"}},
}

func TestAnUnreachableWrapperTakenForThePersonFails(t *testing.T) {
	runDecideControl(t, mutantUnreachableAsPerson, obsDecideShell, obsDecideTool)
}

func TestADecideToolThatSkipsTheWrapperFails(t *testing.T) {
	runDecideControl(t, mutantToolDecidesAsPerson, obsDecideTool, obsDecideClosed)
}

func TestAWrapperDecidingUnverifiedFails(t *testing.T) {
	runDecideControl(t, mutantWrapperDecidesUnverified, obsDecideTool)
}

func runDecideControl(t *testing.T, mutant mutation, breaks ...string) {
	t.Helper()
	if !sandboxSupported() {
		t.Skip("the program has no connect filter for this architecture")
	}
	runFindingsControlOn(t, fixtureColumn.harness, "fixture-decide-sandboxed", playDecideSandboxed, mutant, breaks...)
}
