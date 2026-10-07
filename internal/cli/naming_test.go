package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/state"
)

func TestLaunchRefusesInvalidNamePrefixesWithoutStartingHarness(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(state.DirEnv, dir)
	for _, h := range harness.All() {
		t.Setenv("PATH", launchPath(t, h))
		for _, prefix := range []string{"", "Upper", "../escape", strings.Repeat("x", 32)} {
			code, _, errOut := run("--write", "--name", prefix, h.ID())
			if code != ExitUsage || !strings.Contains(errOut, "prefix") || !strings.Contains(errOut, "full help:") {
				t.Fatalf("prefix=%q code=%d error=%s", prefix, code, errOut)
			}
		}
		code, _, errOut := run("--name=", h.ID())
		if code != ExitUsage || !strings.Contains(errOut, "nonempty prefix") {
			t.Fatalf("empty equals flag: %d %s", code, errOut)
		}
	}
}

func TestLaunchFlowUsesTheAssembledAddress(t *testing.T) {
	steps := flow()
	for i, step := range steps {
		fields := splitExample(step.Command)
		parsed, err := parse(fields[1:])
		if err != nil {
			continue
		} // Help placeholders are not a launch example.
		if parsed.Call.Command == nil || parsed.Call.Command.Harness == nil {
			continue
		}
		prefix := parsed.Call.Flag("name", "")
		address := prefix + "-" + parsed.Call.Command.Harness.ID()
		if i+1 >= len(steps) {
			t.Fatal("launch lacks its matching send")
		}
		next, err := parse(splitExample(steps[i+1].Command)[1:])
		if err != nil || next.Call.Command.Name != "send" || len(next.Call.Positionals) == 0 || next.Call.Positionals[0] != address {
			t.Fatalf("launch %q does not match %q", step.Command, steps[i+1].Command)
		}
	}
}

func TestExplicitPrefixConflictNamesFinalAddress(t *testing.T) {
	for _, h := range harness.All() {
		t.Run(h.ID(), func(t *testing.T) {
			address := "taken-" + h.ID()
			liveSession(t, address)
			// The launch comes from a shell outside any session.
			outsideAnySession(t)
			t.Setenv("PATH", launchPath(t, h))
			code, _, errOut := run("--write", "--name", "taken", h.ID())
			if code != ExitUsage || !strings.Contains(errOut, address) || !strings.Contains(errOut, "prefix") {
				t.Fatalf("conflict=%d %s", code, errOut)
			}
		})
	}
}

// launchPath is the PATH of a launch these tests expect refused by its name: an
// empty one, so no harness can start. A harness that refuses an unreadable
// version before the claim — as a version is read before the name is taken —
// finds a program answering only --version there, so its refusal is the name's
// and not the version's; the program fails the test if it is run for anything
// else.
func launchPath(t *testing.T, h harness.Harness) string {
	t.Helper()
	path := t.TempDir()
	reader, ok := h.(harness.LaunchVersionReader)
	if !ok {
		return path
	}
	if _, err := reader.ReadLaunchVersion(filepath.Join(path, h.ID()), nil, path); err == nil {
		return path
	}
	calls := filepath.Join(t.TempDir(), "calls")
	script := "#!/bin/sh\necho \"$*\" >> " + calls + "\n[ \"$*\" = --version ] && echo 999.0.0\n"
	if err := os.WriteFile(filepath.Join(path, h.ID()), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		raw, _ := os.ReadFile(calls)
		for _, call := range strings.Fields(strings.ReplaceAll(string(raw), " ", "_")) {
			if call != "--version" {
				t.Errorf("%s was started for %q, not only asked its version", h.ID(), call)
			}
		}
	})
	return path
}
