package workflow

import "testing"

// gateException is a capability the gate column may lack: which, on which
// column, why the harness cannot offer it, and the step that retires it. An
// exception is a dated debt, not a second list of what the gate offers.
type gateException struct {
	capability, column, reason, retiredBy string
}

// gateExceptions are every capability the gate column is excused from. Each
// one is checked against the columns as they are, so an exception that no
// longer matches — the gate declares the capability after all, the column is
// not the gate, the capability is not asked about — fails rather than
// lingering as a hole nobody remembers opening.
var gateExceptions = []gateException{
	{
		capability: capabilitySelection,
		column:     fixtureColumn.harness,
		reason:     "selection fencing is the Codex server's own; its observations are recorded only on a column that offers it",
		retiredBy:  "S8",
	},
}

// Exactly one column is the gate, and it is the fixture's: two gates would
// make a red on either block, none would let every red through.
func TestExactlyOneColumnIsTheGate(t *testing.T) {
	var gates []string
	for _, col := range columns {
		if col.gate {
			gates = append(gates, col.harness)
		}
		if isGate(col.harness) != col.gate {
			t.Errorf("isGate(%q)=%v, but the column's gate field is %v", col.harness, isGate(col.harness), col.gate)
		}
	}
	if len(gates) != 1 || gates[0] != fixtureColumn.harness {
		t.Fatalf("the gate columns are %v, want exactly [%s]", gates, fixtureColumn.harness)
	}
	if gateColumn().harness != fixtureColumn.harness {
		t.Errorf("gateColumn() is %q, want %q", gateColumn().harness, fixtureColumn.harness)
	}
}

// The gate column is held to every capability a scenario asks about, but for
// its exceptions. This runs in the five ordinary checks, not only when the
// suite is switched on, because the failure it guards against is silent
// everywhere else: remove one entry from the gate's table and the scenario
// records that observation as unsupported, which a review once watched come
// out green.
func TestTheGateColumnDeclaresEveryCapability(t *testing.T) {
	if len(knownCapabilities) == 0 {
		t.Fatal("no capability was declared, so this test would pass on nothing")
	}
	// Every exception names the gate column; TestEveryGateExceptionStillMatches
	// holds that, so one on another column fails there rather than here.
	gate := gateColumn()
	excused := map[string]bool{}
	for _, exception := range gateExceptions {
		excused[exception.capability] = true
	}
	for _, name := range knownCapabilities {
		if !gate.offers(name) && !excused[name] {
			t.Errorf("the gate column %s does not declare %q and no exception names it; every observation there has to be made", gate.harness, name)
		}
	}
}

// An exception matches the columns as they are, and says why and until when.
func TestEveryGateExceptionStillMatches(t *testing.T) {
	gate := gateColumn()
	known := map[string]bool{}
	for _, name := range knownCapabilities {
		known[name] = true
	}
	seen := map[string]bool{}
	for _, exception := range gateExceptions {
		key := exception.column + "/" + exception.capability
		switch {
		case seen[key]:
			t.Errorf("the exception for %s is written twice", key)
		case !known[exception.capability]:
			t.Errorf("the exception for %s names a capability no scenario asks about", key)
		case exception.column != gate.harness:
			t.Errorf("the exception for %s names a column that is not the gate (%s)", key, gate.harness)
		case gate.offers(exception.capability):
			t.Errorf("the exception for %s excuses a capability the gate declares", key)
		case exception.reason == "" || exception.retiredBy == "":
			t.Errorf("the exception for %s does not say why, or which step retires it", key)
		}
		seen[key] = true
	}
}

// An unsupported outcome is acceptable off the gate and a failure on it. Both
// halves, over every column, because either alone is one line away from
// letting the gate lose an observation or making a search column impossible
// to finish.
func TestUnsupportedIsAFailureOnlyOnTheGate(t *testing.T) {
	for _, col := range columns {
		c := &Case{spec: Spec{Harness: col.harness}}
		if got := c.acceptable(Unsupported); got != !col.gate {
			t.Errorf("unsupported on %s acceptable=%v, want %v", col.harness, got, !col.gate)
		}
		if !c.acceptable(Pass) {
			t.Errorf("pass on %s was not acceptable", col.harness)
		}
	}
}
