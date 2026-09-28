package workflow

import "testing"

// The controls of grant-forgery, each the product with one check of a
// grant's origin undone.

// Main's wrapper takes a grant from any process of the user, not only from
// one below it: a registration written straight to its address is taken.
// rewake send itself still registers only with a listener above it, so the
// forged sends stay refused.
var mutantGrantFromAnywhere = mutation{
	name:  "grant-from-anywhere",
	file:  "internal/grantauth/server.go",
	edits: []edit{{"if err := proc.Default.Descends(int(peer.Pid), a.Self); err != nil {", "if err := proc.Default.Descends(int(peer.Pid), a.Self); false && err != nil {"}},
}

// The receiving wrapper checks a grant's directories and never asks main:
// both letters written by hand reach the roots.
var mutantGrantUnconfirmed = mutation{
	name:  "grant-unconfirmed",
	file:  "internal/wrap/grants.go",
	edits: []edit{{"if err := confirmGrant(dir, name, epoch, \"\", message); err != nil", "if err := error(nil); err != nil"}},
}

// A confirmation counts from whichever process serves the address: the
// listener in main's place is believed.
var mutantGrantAnyListener = mutation{
	name:  "grant-any-listener",
	file:  "internal/grantauth/grantauth.go",
	edits: []edit{{"if int(peer.Pid) != e.PID || e.PID <= 0 {", "if false && (int(peer.Pid) != e.PID || e.PID <= 0) {"}},
}

func TestAGrantRegisteredFromAnyProcessFails(t *testing.T) {
	runGrantForgeryControl(t, mutantGrantFromAnywhere, obsRawRegister)
}

func TestAGrantNeverConfirmedWithMainFails(t *testing.T) {
	runGrantForgeryControl(t, mutantGrantUnconfirmed, obsRawRegister, obsHandLetter, obsForeignAnswer)
}

func TestAGrantConfirmedByAnyListenerFails(t *testing.T) {
	runGrantForgeryControl(t, mutantGrantAnyListener, obsForeignAnswer)
}

func runGrantForgeryControl(t *testing.T, mutant mutation, breaks ...string) {
	t.Helper()
	runFindingsControlOn(t, codexColumn.harness, "codex-grant-forgery", playGrantForgery, mutant, breaks...)
}
