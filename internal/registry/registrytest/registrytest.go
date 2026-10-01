// Package registrytest helps tests describe runs of this build.
package registrytest

import (
	"testing"

	"github.com/praline-labs/rewake/internal/registry"
)

// Boot is the id of the boot the test runs in. A session a test publishes
// needs it to be a run of this build: without it the run reads as the earlier
// build's, whose mail this build holds rather than delivers.
func Boot(t testing.TB) string {
	t.Helper()
	boot, err := registry.CurrentBoot()
	if err != nil {
		t.Fatalf("the boot id cannot be read: %v", err)
	}
	return boot
}
