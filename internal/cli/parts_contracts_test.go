package cli

import (
	"maps"
	"strconv"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/host/mainhost"
	"github.com/praline-labs/rewake/internal/host/spacehost"
	"github.com/praline-labs/rewake/internal/host/stream"
	"github.com/praline-labs/rewake/internal/infra/parts"
)

// A switch compares every contract this build speaks, so a contract a host
// names is held when a change of its value is a change the switch refuses.
// The enumeration is the build's own, read from the plan's comparison, not a
// list of names kept here.
func TestEveryContractTheBuildSpeaksIsWhatASwitchCompares(t *testing.T) {
	running := ownPart(testCatalog())
	for name := range running.Speaks {
		installed := running
		installed.Speaks = maps.Clone(running.Speaks)
		installed.Speaks[name] += ".next"
		if why := parts.Admits(running, installed); !strings.Contains(why, name) {
			t.Errorf("a changed %s is admitted: %q", name, why)
		}
	}
}

// The hosts that carry a relay speak their handover's version together with
// the screen file's: a raise of the screen's version alone is a contract the
// switch refuses.
func TestTheHostContractsCarryTheScreensVersion(t *testing.T) {
	speaks := contractsOf(testCatalog()).Speaks
	for name, want := range map[string]string{
		spacehost.ContractName: stream.Contract(spacehost.HandoverVersion),
		mainhost.ContractName:  stream.Contract(mainhost.HandoverVersion),
	} {
		if speaks[name] != want {
			t.Errorf("%s is %q, want %q", name, speaks[name], want)
		}
		if !strings.HasSuffix(want, "."+strconv.Itoa(stream.ScreenVersion)) {
			t.Errorf("%s names no screen version: %q", name, want)
		}
	}
}
