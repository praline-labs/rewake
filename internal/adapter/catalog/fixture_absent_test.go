//go:build !rewaketest

package catalog

import (
	"testing"
)

// A build without the fixture's tag — a release — has no fixture to launch.
func TestTheFixtureIsAbsentWithoutItsTag(t *testing.T) {
	for _, h := range New().All() {
		if h.ID() == "fixture" {
			t.Fatal("the fixture is in a catalog built without rewaketest")
		}
	}
}
