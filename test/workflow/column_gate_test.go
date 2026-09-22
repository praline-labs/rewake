package workflow

import "testing"

// The gate column is held to every capability a scenario asks about. This runs
// in the five ordinary checks, not only when the suite is switched on, because
// the failure it guards against is silent everywhere else: remove one entry
// from the gate's table and the scenario records that observation as
// unsupported, which a review once watched come out green.
func TestTheGateColumnDeclaresEveryCapability(t *testing.T) {
	if len(knownCapabilities) == 0 {
		t.Fatal("no capability was declared, so this test would pass on nothing")
	}
	for _, name := range knownCapabilities {
		if !codexColumn.offers(name) {
			t.Errorf("the gate column does not declare %q; every observation there has to be made", name)
		}
	}
	if !isGate(codexColumn.harness) {
		t.Error("the Codex column is not treated as the gate")
	}
	if isGate(claudeColumn.harness) {
		t.Error("the search column is treated as the gate")
	}
}

// An unsupported outcome is acceptable off the gate and a failure on it. Both
// halves, because either alone is one line away from letting the gate lose an
// observation or making the search column impossible to finish.
func TestUnsupportedIsAFailureOnlyOnTheGate(t *testing.T) {
	for _, tc := range []struct {
		harness    string
		acceptable bool
	}{
		{codexColumn.harness, false},
		{claudeColumn.harness, true},
	} {
		c := &Case{spec: Spec{Harness: tc.harness}}
		if got := c.acceptable(Unsupported); got != tc.acceptable {
			t.Errorf("unsupported on %s acceptable=%v, want %v", tc.harness, got, tc.acceptable)
		}
		if !c.acceptable(Pass) {
			t.Errorf("pass on %s was not acceptable", tc.harness)
		}
	}
}
