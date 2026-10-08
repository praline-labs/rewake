package workflow

import "testing"

// The controls of grant-dir, each the product with one part of the grant
// undone.

// A grant goes into a running turn, as --grant-git did before the wait.
var mutantGrantSteered = mutation{
	name:  "grant-steered",
	file:  "internal/harness/codex/server_delivery.go",
	edits: []edit{{`if read.err == nil && read.status == "active" {`, `if false && read.err == nil && read.status == "active" {`}},
}

// The granted directory is journaled and reported but never added to the
// roots. The revocation then finds it gone and records it dropped, so the
// observation of taking it back breaks too.
var mutantGrantNotAdded = mutation{
	name:  "grant-not-added",
	file:  "internal/harness/codex/server_dirgrant.go",
	edits: []edit{{"			if !hasRoot(*roots, directory) {\n				*roots = append(*roots, directory)\n", "			if false && !hasRoot(*roots, directory) {\n				*roots = append(*roots, directory)\n"}},
}

// A grant whose task is reported on stays in the roots.
var mutantGrantKept = mutation{
	name:  "grant-kept",
	file:  "internal/harness/codex/server_dirgrant.go",
	edits: []edit{{"			roots = slices.Delete(roots, at, at+1)\n", ""}},
}

func TestAGrantSteeredIntoARunningTurnFails(t *testing.T) {
	runGrantDirControl(t, mutantGrantSteered, obsGrantWaitsIdle)
}

func TestAGrantNeverAddedToTheRootsFails(t *testing.T) {
	runGrantDirControl(t, mutantGrantNotAdded, obsGrantReachRoots, obsGrantTakenBack)
}

func TestAGrantKeptAfterItsReportFails(t *testing.T) {
	runGrantDirControl(t, mutantGrantKept, obsGrantTakenBack)
}

func runGrantDirControl(t *testing.T, mutant mutation, breaks ...string) {
	t.Helper()
	runFindingsControlOn(t, codexColumn.harness, "codex-grant-dir", playGrantDir, mutant, breaks...)
}
