package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
)

// A launch reads the installed version before the claim: one at or above the
// minimum is accepted, one below it is refused with both versions named.
func TestAVersionBelowTheMinimumIsRefused(t *testing.T) {
	for version, refused := range map[string]bool{"2.1.280": true, "2.0.999": true, "2.1.287": false, "2.1.300": false, "2.2.0": false, "3.0.0": false} {
		program := versionScript(t, "#!/bin/sh\necho '"+version+" (Claude Code)'\n")
		read, err := New().(harness.LaunchVersionReader).ReadLaunchVersion(program, os.Environ(), t.TempDir())
		if (err != nil) != refused {
			t.Errorf("%s: refused %v: %v", version, err != nil, err)
		}
		if !refused && read.Value != version {
			t.Errorf("%s: read %q", version, read.Value)
		}
	}
	program := versionScript(t, "#!/bin/sh\necho '2.1.280 (Claude Code)'\n")
	_, err := New().(harness.LaunchVersionReader).ReadLaunchVersion(program, os.Environ(), t.TempDir())
	if err == nil || err.Error() != "Claude Code 2.1.280 is installed; rewake needs 2.1.287 or later" {
		t.Errorf("the refusal says %v", err)
	}
}

// A version that cannot be read refuses the launch, saying how it was asked
// and what came of it.
func TestAnUnreadableVersionIsRefused(t *testing.T) {
	for name, c := range map[string]struct{ script, says string }{
		"no version": {"#!/bin/sh\necho 'Claude Code'\n", "claude --version printed no version"},
		"failing":    {"#!/bin/sh\necho '2.1.287 (Claude Code)'\nexit 3\n", "claude --version ended with"},
		"missing":    {"", "claude --version"},
	} {
		program := filepath.Join(t.TempDir(), "claude")
		if c.script != "" {
			program = versionScript(t, c.script)
		}
		_, err := New().(harness.LaunchVersionReader).ReadLaunchVersion(program, os.Environ(), t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "could not be read") || !strings.Contains(err.Error(), c.says) || !strings.Contains(err.Error(), "2.1.287 or later") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func versionScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
