package launch

import (
	"os"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
)

// runtimeScript is a runtime that prints runtime for --version and version when
// it runs the shipped entry with --version, as node runs pi's script.
func runtimeScript(t *testing.T, runtime, version string) adapter.Start {
	t.Helper()
	program := versionScript(t, "#!/bin/sh\nif [ \"$1\" = --version ]; then echo v"+runtime+"; else echo "+version+"; fi\n")
	return adapter.Start{
		Command: program, Args: []string{"/shipped/cli.js"},
		RuntimeTitle: "Runtime", Runtime: adapter.VersionGate{Minimum: "22.19.0", Args: []string{"--version"}},
	}
}

func pinned() gated {
	return gated{gate: adapter.VersionGate{Minimum: "1.2.0", Args: []string{"--version"}, Pinned: "1.3.0"}}
}

// The shipped copy runs only at its pinned version, on a runtime at or above
// its minimum: an older runtime refuses before the copy is asked, and a copy
// newer or older than the pin refuses as one below a minimum would.
func TestTheShippedCopyRunsOnlyAtItsPinnedVersion(t *testing.T) {
	for _, c := range []struct{ runtime, version, refusal string }{
		{"22.19.0", "1.3.0", ""},
		{"24.1.0", "1.3.0", ""},
		{"22.18.9", "1.3.0", "Runtime 22.18.9 is installed; rewake needs 22.19.0 or later"},
		{"22.19.0", "1.2.0", "Stub harness 1.2.0 was found where rewake's own copy should be; this rewake runs Stub harness 1.3.0"},
		{"22.19.0", "1.4.0", "Stub harness 1.4.0 was found"},
		{"22.19.0", "none", "the version of the Stub harness shipped with rewake could not be read"},
	} {
		read, err := GateShipped(pinned(), runtimeScript(t, c.runtime, c.version), os.Environ(), t.TempDir())
		switch {
		case c.refusal == "" && (err != nil || read != c.version):
			t.Errorf("%s on %s: read %q, %v", c.version, c.runtime, read, err)
		case c.refusal != "" && (err == nil || !strings.Contains(err.Error(), c.refusal)):
			t.Errorf("%s on %s: the refusal says %v", c.version, c.runtime, err)
		}
	}
}
