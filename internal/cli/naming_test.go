package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

func TestLaunchRefusesInvalidNamePrefixesWithoutStartingHarness(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(state.DirEnv, dir)
	t.Setenv("PATH", t.TempDir())
	for _, h := range harness.All() {
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
			t.Setenv("PATH", t.TempDir())
			code, _, errOut := run("--write", "--name", "taken", h.ID())
			if code != ExitUsage || !strings.Contains(errOut, address) || !strings.Contains(errOut, "prefix") {
				t.Fatalf("conflict=%d %s", code, errOut)
			}
		})
	}
}
