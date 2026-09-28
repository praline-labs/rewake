package claude

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/role"
)

// Claude Code takes both settings as ordinary flags, so the adapter passes
// them as flags — and leaves them alone when the caller named their own.
func TestClaudeLaunchTakesModelAndEffortFromTheEnvironment(t *testing.T) {
	t.Setenv("REWAKE_CLAUDE_MODEL", "configured-model")
	t.Setenv("REWAKE_CLAUDE_EFFORT", "low")

	plan, err := New().Launch(harness.LaunchRequest{Name: "worker", Dir: t.TempDir(), Socket: socketPath(t), Role: role.General})
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Join(plan.Args, " ")
	if !strings.Contains(line, "--model configured-model") || !strings.Contains(line, "--effort low") {
		t.Errorf("the configured defaults were not passed: %v", plan.Args)
	}
	if len(plan.Notes) == 0 {
		t.Error("the substitution was not announced")
	}
}

func TestClaudeLaunchLeavesTheCallersOwnChoiceAlone(t *testing.T) {
	t.Setenv("REWAKE_CLAUDE_MODEL", "configured-model")
	t.Setenv("REWAKE_CLAUDE_EFFORT", "low")

	given := []string{"--model", "asked-for", "--effort", "high"}
	plan, err := New().Launch(harness.LaunchRequest{Name: "worker", Dir: t.TempDir(), Socket: socketPath(t), Args: given, Role: role.General})
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Join(plan.Args, " ")
	if strings.Contains(line, "configured-model") || strings.Contains(line, "--effort low") {
		t.Errorf("a default overrode what the caller asked for: %v", plan.Args)
	}
	for _, note := range plan.Notes {
		if strings.Contains(note, "REWAKE_CLAUDE_") {
			t.Errorf("a note claimed a substitution that did not happen: %q", note)
		}
	}
}

func socketPath(t *testing.T) string {
	t.Helper()
	return t.TempDir() + "/s.sock"
}
