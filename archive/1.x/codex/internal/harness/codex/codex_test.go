package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
)

// codexHome writes a Codex home with the given config.toml content.
func codexHome(t *testing.T, config string) string {
	t.Helper()
	home := t.TempDir()
	if config != "" {
		if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
	}
	t.Setenv("CODEX_HOME", home)
	return home
}

func configValue(args []string, key string) (string, bool) {
	for index, arg := range args {
		if arg == configFlag && index+1 < len(args) && strings.HasPrefix(args[index+1], key+"=") {
			return strings.TrimPrefix(args[index+1], key+"="), true
		}
	}
	return "", false
}

func TestIntroIsPassedForOneLaunch(t *testing.T) {
	codexHome(t, "")

	plan, err := New().Launch(harness.LaunchRequest{Name: "web", Dir: t.TempDir(), Intro: true})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}

	value, ok := configValue(plan.Args, introKey)
	if !ok {
		t.Fatalf("the briefing was not passed: %v", plan.Args)
	}
	if !strings.Contains(value, `web`) || !strings.Contains(value, "rewake guide") {
		t.Errorf("briefing = %s, want the session name and where the instructions are", value)
	}
}

func TestIntroIsSkippedWhenNotWanted(t *testing.T) {
	codexHome(t, "")

	plan, err := New().Launch(harness.LaunchRequest{Name: "web", Dir: t.TempDir(), Intro: false})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if _, ok := configValue(plan.Args, introKey); ok {
		t.Errorf("--no-intro still sent a briefing: %v", plan.Args)
	}
}

func TestCallerConfigWins(t *testing.T) {
	codexHome(t, "")

	plan, err := New().Launch(harness.LaunchRequest{
		Name:  "web",
		Dir:   t.TempDir(),
		Intro: true,
		Args:  []string{configFlag, introKey + `="mine"`},
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if count := strings.Count(strings.Join(plan.Args, " "), introKey+"="); count != 1 {
		t.Errorf("the key was set %d times, want the caller's only: %v", count, plan.Args)
	}
}

func TestWritableRootsAreLeftAloneByDefault(t *testing.T) {
	codexHome(t, "[sandbox_workspace_write]\nnetwork_access = true\n")

	plan, err := New().Launch(harness.LaunchRequest{Name: "web", Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if _, ok := configValue(plan.Args, "sandbox_workspace_write.writable_roots"); ok {
		t.Errorf("the sandbox configuration was changed for no reason: %v", plan.Args)
	}
}
