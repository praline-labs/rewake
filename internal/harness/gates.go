package harness

import (
	"fmt"
	"slices"
	"strings"
)

// The gates of the mail tool's launch (docs/mail-bridge-launch.md#gates): live
// or source checks the stage's rules wait for. While a gate is open the rules'
// action for what it leaves unknown applies — a refusal, no tool, or no read
// acknowledgment — and a gate is closed only by being recorded below for the
// harness version its check ran against.
const (
	GateG1 = "G1"
	GateG2 = "G2"
	GateG3 = "G3"
	GateG4 = "G4"
	GateG5 = "G5"
	GateG6 = "G6"
	GateG7 = "G7"
	GateG8 = "G8"
	GateG9 = "G9"
	GateL4 = "L4"
	GateL5 = "L5"
	GateS1 = "S1"
)

// gateNames are every gate of the table, in its order.
var gateNames = []string{GateG1, GateG2, GateG3, GateG4, GateG5, GateG6, GateG7, GateG8, GateG9, GateL4, GateL5, GateS1}

// closedGates records each gate a check closed: gate, harness, the versions
// it ran against. A version the check did not run against keeps the gate
// open, since what it settled is a property of that version.
var closedGates = map[string]map[string][]string{}

// GatesAssumedEnv names gates a launch takes as closed without their check.
// It exists for the live checks of the stage, which cannot run while the
// gates they close keep the tool out (docs/mail-bridge-launch.md#gates);
// every assumed gate is said at launch and kept in the session record.
const GatesAssumedEnv = "REWAKE_GATES_ASSUMED"

// Gates is what one launch takes for each gate.
type Gates struct {
	assumed []string
	closed  map[string]bool
}

// ParseAssumedGates reads REWAKE_GATES_ASSUMED: gate names separated by
// commas. A name the table does not hold is an error, never ignored: a
// misspelled gate would leave the run testing something other than it says.
func ParseAssumedGates(value string) ([]string, error) {
	var names []string
	for _, field := range strings.Split(value, ",") {
		name := strings.ToUpper(strings.TrimSpace(field))
		if name == "" {
			continue
		}
		if !slices.Contains(gateNames, name) {
			return nil, fmt.Errorf("%s names %q, which is not a gate; the gates are %s", GatesAssumedEnv, name, strings.Join(gateNames, ", "))
		}
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names, nil
}

// ResolveGates is the gates of one launch of a harness at a version, "" when
// it was not read; assumed are the names ParseAssumedGates returned.
func ResolveGates(harnessID, version string, assumed []string) Gates {
	gates := Gates{assumed: append([]string(nil), assumed...), closed: map[string]bool{}}
	for gate, byHarness := range closedGates {
		if version != "" && slices.Contains(byHarness[harnessID], version) {
			gates.closed[gate] = true
		}
	}
	return gates
}

// GatesNeedVersion says whether any gate is recorded closed for a harness:
// only then is its version worth reading at launch.
func GatesNeedVersion(harnessID string) bool {
	for _, byHarness := range closedGates {
		if len(byHarness[harnessID]) > 0 {
			return true
		}
	}
	return false
}

// Open says whether the rules' action for what a gate leaves unknown
// applies to this launch.
func (g Gates) Open(name string) bool {
	return !g.closed[name] && !slices.Contains(g.assumed, name)
}

// Assumed lists the gates this launch takes as closed without their check.
func (g Gates) Assumed() []string { return append([]string(nil), g.assumed...) }
