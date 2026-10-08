package workflow

import "testing"

// The controls of fixture-grant-dir, each the core's part of a grant with one
// step undone. The controls of an adapter's own part — waiting for an idle
// session, adding the directory and taking it out — left with the Codex
// adapter they mutated (archive/1.x/codex/README.md) and return with the
// Permissions capability.

// The receiving wrapper takes the directories as they were when sent: the
// one removed meanwhile is offered all the same.
var mutantGrantNotRechecked = mutation{
	name:  "grant-not-rechecked",
	file:  "internal/wrap/grants.go",
	edits: []edit{{"if err := rules.Recheck(directory, slices.Contains(message.GrantBroad, directory)); err != nil {", "if err := rules.Recheck(directory, slices.Contains(message.GrantBroad, directory)); false && err != nil {"}},
}

// A grant that fails leaves its sender untold: the task is gone and nobody
// works on it.
var mutantGrantRefusalUntold = mutation{
	name:  "grant-refusal-untold",
	file:  "internal/inbox/grants.go",
	edits: []edit{{"if result.State == Failed && !result.Withdrawn && CarriesGrant(message) {", "if false && result.State == Failed && !result.Withdrawn && CarriesGrant(message) {"}},
}

// send writes the letter without registering its grant: no wrapper confirms
// it, so every grant main sends fails, the one meant to wait included, and
// main holds nothing to forget.
var mutantGrantUnregistered = mutation{
	name:  "grant-unregistered",
	file:  "internal/cli/send_grant.go",
	edits: []edit{{"\tif !inbox.CarriesGrant(message) {\n\t\treturn nil\n\t}", "\tif true || !inbox.CarriesGrant(message) {\n\t\treturn nil\n\t}"}},
}

// Main's wrapper keeps a grant whose task was reported on until its lifetime
// runs out.
var mutantGrantHeldAfterReport = mutation{
	name:  "grant-held-after-report",
	file:  "internal/grantauth/server.go",
	edits: []edit{{"\t\t\t\tif !open {\n", "\t\t\t\tif false && !open {\n"}},
}

func TestAGrantNotCheckedAgainAtDeliveryFails(t *testing.T) {
	runFixtureGrantControl(t, mutantGrantNotRechecked, obsFixtureGrantRechecked)
}

func TestAGrantRefusedWithoutTellingItsSenderFails(t *testing.T) {
	runFixtureGrantControl(t, mutantGrantRefusalUntold, obsFixtureGrantRechecked)
}

func TestAGrantSentUnregisteredFails(t *testing.T) {
	runFixtureGrantControl(t, mutantGrantUnregistered, obsFixtureGrantOffered, obsFixtureGrantLifetime, obsFixtureGrantRechecked)
}

func TestAGrantHeldAfterItsReportFails(t *testing.T) {
	runFixtureGrantControl(t, mutantGrantHeldAfterReport, obsFixtureGrantLifetime)
}

func runFixtureGrantControl(t *testing.T, mutant mutation, breaks ...string) {
	t.Helper()
	runFindingsControlOn(t, fixtureColumn.harness, "fixture-grant-dir", playFixtureGrantDir, mutant, breaks...)
}
