//go:build rewakefixture

package catalog

import (
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/harness/fixture"
)

// A build with the fixture's tag has it beside the others, once.
func TestTheFixtureIsPresentWithItsTag(t *testing.T) {
	found := 0
	for _, h := range harness.All() {
		if h.ID() == fixture.ID {
			found++
		}
	}
	if found != 1 || len(harness.All()) < 3 {
		t.Fatalf("the fixture is in the catalog %d times among %d harnesses", found, len(harness.All()))
	}
}
