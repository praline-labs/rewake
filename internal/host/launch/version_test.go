package launch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
)

// gated is an adapter as far as a launch's version read sees it: a title, and
// a gate.
type gated struct {
	adapter.Harness
	gate adapter.VersionGate
}

func (g gated) Title() string                { return "Stub harness" }
func (g gated) Version() adapter.VersionGate { return g.gate }
func (g gated) EnvNotInherited() []string    { return nil }

// stub is gated at 2.1.287, asked with --version.
func stub() gated {
	return gated{gate: adapter.VersionGate{Minimum: "2.1.287", Args: []string{"--version"}}}
}

func versionScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stub")
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// A launch reads the installed version before the claim: one at or above the
// minimum is accepted, one below it is refused with both versions named.
func TestAVersionBelowTheMinimumIsRefused(t *testing.T) {
	for version, refused := range map[string]bool{"2.1.280": true, "2.0.999": true, "2.1.287": false, "2.1.300": false, "2.2.0": false, "3.0.0": false} {
		program := versionScript(t, "#!/bin/sh\necho '"+version+" (Stub)'\n")
		read, err := Gate(stub(), program, os.Environ(), t.TempDir())
		if (err != nil) != refused {
			t.Errorf("%s: refused %v: %v", version, err != nil, err)
		}
		if !refused && read != version {
			t.Errorf("%s: read %q", version, read)
		}
	}
	program := versionScript(t, "#!/bin/sh\necho '2.1.280 (Stub)'\n")
	_, err := Gate(stub(), program, os.Environ(), t.TempDir())
	if err == nil || err.Error() != "Stub harness 2.1.280 is installed; rewake needs 2.1.287 or later" {
		t.Errorf("the refusal says %v", err)
	}
}

// A version that cannot be read refuses the launch, saying how it was asked
// and what came of it.
func TestAnUnreadableVersionIsRefused(t *testing.T) {
	for name, c := range map[string]struct{ script, says string }{
		"no version": {"#!/bin/sh\necho 'Stub'\n", "stub --version printed no version"},
		"failing":    {"#!/bin/sh\necho '2.1.287 (Stub)'\nexit 3\n", "stub --version ended with"},
		"missing":    {"", "stub --version"},
	} {
		program := filepath.Join(t.TempDir(), "stub")
		if c.script != "" {
			program = versionScript(t, c.script)
		}
		_, err := Gate(stub(), program, os.Environ(), t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "could not be read") || !strings.Contains(err.Error(), c.says) || !strings.Contains(err.Error(), "2.1.287 or later") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// L2: the version's refusal is a diagnostic of the launch, and says how the
// version was asked, never what the program printed.
func TestAVersionRefusalNeverCarriesWhatTheProgramPrinted(t *testing.T) {
	const marker = "secret-marker-7f3a"
	program := versionScript(t, "#!/bin/sh\necho stub "+marker+"\n")
	if _, err := Gate(stub(), program, os.Environ(), t.TempDir()); err == nil || strings.Contains(err.Error(), marker) {
		t.Errorf("the version's refusal: %v", err)
	}
}

// A gate without a minimum reads nothing: the program is never started.
func TestAGateWithoutAMinimumReadsNothing(t *testing.T) {
	program := filepath.Join(t.TempDir(), "absent")
	if read, err := Gate(gated{}, program, os.Environ(), t.TempDir()); err != nil || read != "" {
		t.Errorf("read %q, %v", read, err)
	}
}
