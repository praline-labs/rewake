package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/role"
)

// privateHome keeps a test's worker launch from reading the person's own
// worker profile.
func privateHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

// profileLaunch plans a launch of the role in a private home, whose worker
// profile directory the test may fill first.
func profileLaunch(t *testing.T, home string, part role.Role, command string, env []string, args ...string) (adapter.LaunchPlan, error) {
	t.Helper()
	t.Setenv("HOME", home)
	return New().Launch(adapter.LaunchRequest{
		Name: "api", Dir: t.TempDir(), Args: args, Intro: true, Role: part, Command: command,
		Socket: filepath.Join(t.TempDir(), "api.sock"), Env: append([]string{"HOME=" + home}, env...),
	})
}

func settingsOf(t *testing.T, args []string) map[string]any {
	t.Helper()
	values := adapter.FlagValues(args, settingsFlag)
	if len(values) != 1 {
		t.Fatalf("want one %s, got %v in %q", settingsFlag, values, args)
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(values[0]), &settings); err != nil {
		t.Fatalf("settings are not JSON: %v", err)
	}
	return settings
}

func TestWorkerProfileDefaults(t *testing.T) {
	home := t.TempDir()
	for _, part := range []role.Role{role.General, role.Write} {
		plan, err := profileLaunch(t, home, part, "", []string{"CLAUDE_CONFIG_DIR=/elsewhere"})
		if err != nil {
			t.Fatal(err)
		}
		if got := adapter.FlagValues(plan.Args, settingSourcesFlag); !slices.Equal(got, []string{workerSources}) {
			t.Fatalf("%s: setting sources %v", part.ID, got)
		}
		settings := settingsOf(t, plan.Args)
		excludes := []any{"/elsewhere/CLAUDE.md", filepath.Join(home, ".claude", "CLAUDE.md")}
		if settings["autoMemoryEnabled"] != false || !slices.Equal(settings["claudeMdExcludes"].([]any), excludes) {
			t.Fatalf("%s: settings %v", part.ID, settings)
		}
		if adapter.HasFlag(plan.Args, mcpConfigFlag) || len(adapter.FlagValues(plan.Args, pluginDirFlag)) != 0 {
			t.Fatalf("%s: an empty profile added its parts: %q", part.ID, plan.Args)
		}
	}
}

func TestMainGetsNoProfile(t *testing.T) {
	plan, err := profileLaunch(t, t.TempDir(), role.Main, "", nil, "--settings", "mine.json")
	if err != nil {
		t.Fatal(err)
	}
	if adapter.HasFlag(plan.Args, settingSourcesFlag) || !slices.Equal(adapter.FlagValues(plan.Args, settingsFlag), []string{"mine.json"}) {
		t.Fatalf("main's launch changed: %q", plan.Args)
	}
}

func TestWorkerProfileOfThePerson(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "rewake", "claude", "worker")
	if err := os.MkdirAll(filepath.Join(dir, "plugins"), 0o700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"settings.json": `{"autoMemoryEnabled": true, "language": "x", "claudeMdExcludes": ["/other/CLAUDE.md"], "env": {"DISABLE_FEEDBACK_COMMAND": "0", "OWN": "1"}}`,
		"mcp.json":      `{"mcpServers": {}}`,
		"CLAUDE.md":     "\nWorker rules.\n",
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := profileLaunch(t, home, role.Write, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	settings := settingsOf(t, plan.Args)
	env := settings["env"].(map[string]any)
	if settings["autoMemoryEnabled"] != true || settings["language"] != "x" || env["DISABLE_AUTOUPDATER"] != "1" || env["DISABLE_FEEDBACK_COMMAND"] != "0" || env["OWN"] != "1" {
		t.Fatalf("merged settings: %v", settings)
	}
	if excludes := settings["claudeMdExcludes"].([]any); len(excludes) != 2 || excludes[1] != "/other/CLAUDE.md" {
		t.Fatalf("the person's exclude replaced rewake's: %v", excludes)
	}
	if !slices.Contains(adapter.FlagValues(plan.Args, pluginDirFlag), filepath.Join(dir, "plugins")) || !slices.Equal(adapter.FlagValues(plan.Args, mcpConfigFlag), []string{filepath.Join(dir, "mcp.json")}) {
		t.Fatalf("plugins or MCP servers missing: %q", plan.Args)
	}
	intro := adapter.FlagValues(plan.Args, introFlag)
	if len(intro) != 1 || !strings.HasSuffix(intro[0], "\n\nWorker rules.") || !strings.Contains(intro[0], "rewake") {
		t.Fatalf("instructions after the briefing: %q", intro)
	}
}

func TestWorkerProfileRefusals(t *testing.T) {
	home := t.TempDir()
	for _, flag := range []string{settingsFlag, settingSourcesFlag} {
		if _, err := profileLaunch(t, home, role.Write, "", nil, flag, "x"); err == nil || !strings.Contains(err.Error(), "settings.json") {
			t.Fatalf("a worker's %s: %v", flag, err)
		}
	}
	dir := filepath.Join(home, ".config", "rewake", "claude", "worker")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("[1]"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := profileLaunch(t, home, role.Write, "", nil); err == nil || !strings.Contains(err.Error(), "not a JSON object") {
		t.Fatalf("a broken settings.json: %v", err)
	}
}

// A wrapper decides the configuration it starts Claude Code with; the
// profile stays out of its way, as a wrapper's worker launched before it did.
func TestWorkerProfileUnderCommand(t *testing.T) {
	plan, err := profileLaunch(t, t.TempDir(), role.Write, "my-claude", nil, "--settings", "mine.json")
	if err != nil {
		t.Fatal(err)
	}
	if adapter.HasFlag(plan.Args, settingSourcesFlag) || !slices.Equal(adapter.FlagValues(plan.Args, settingsFlag), []string{"mine.json"}) || !strings.Contains(strings.Join(plan.Notes, "\n"), "--command") {
		t.Fatalf("the profile reached a wrapper's launch: %q %q", plan.Args, plan.Notes)
	}
}

func TestContextWindow(t *testing.T) {
	for value, ok := range map[string]bool{
		"auto": true, "100k": true, "300K": true, "1M": true, "1m": true, "200000": true, "1000000": true,
		"99k": false, "2M": false, "1.5M": false, "200": false, "300K (note)": false, "": false, "k": false, "-300k": false, "+300k": false,
	} {
		if err := checkWindow(value); (err == nil) != ok {
			t.Errorf("checkWindow(%q) = %v", value, err)
		}
	}
	args, err := New().(claudeHarness).LaunchSettings("", "", "300k")
	if err != nil || !slices.Equal(args, []string{autocompactFlag, "300k"}) {
		t.Fatalf("context as a flag: %v, %v", args, err)
	}
}
