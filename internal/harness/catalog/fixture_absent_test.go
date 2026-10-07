//go:build !rewakefixture

package catalog

import (
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
)

// A build without the fixture's tag — a release — has no fixture to launch.
func TestTheFixtureIsAbsentWithoutItsTag(t *testing.T) {
	for _, h := range harness.All() {
		if h.ID() == "fixture" {
			t.Fatal("the fixture is in a catalog built without rewakefixture")
		}
	}
}
