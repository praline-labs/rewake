package claude

import (
	"slices"
	"testing"
)

// The adapter's gate names the oldest Claude Code it starts and asks the
// installed one with --version; the host reads it before the claim
// (internal/host/launch, TestAVersionBelowTheMinimumIsRefused).
func TestTheGateNamesTheMinimumAndHowToAsk(t *testing.T) {
	gate := New().Version()
	if gate.Minimum != "2.1.287" || !slices.Equal(gate.Args, []string{"--version"}) {
		t.Errorf("gate = %+v", gate)
	}
}
