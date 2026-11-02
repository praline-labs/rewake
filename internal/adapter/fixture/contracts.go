//go:build rewaketest

package fixture

import "strconv"

// Contracts is the fixture's handed state, which a switch carries to the
// target's image (adapter.Contracts).
func (fixtureHarness) Contracts() map[string]string {
	return map[string]string{"handover." + ID: strconv.Itoa(handoverVersion)}
}
