//go:build rewakefixture

package catalog

import (
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/adapter/fixture"
)

// A build with the fixture's tag has it beside the others, once.
func TestTheFixtureIsPresentWithItsTag(t *testing.T) {
	found := 0
	for _, h := range adapter.All() {
		if h.ID() == fixture.ID {
			found++
		}
	}
	if found != 1 || len(adapter.All()) < 2 {
		t.Fatalf("the fixture is in the catalog %d times among %d harnesses", found, len(adapter.All()))
	}
}
