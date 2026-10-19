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
		if deny := settings["permissions"].(map[string]any)["deny"].([]any); len(deny) != len(deniedTools) || deny[0] != "Agent" {
			t.Fatalf("%s: the tools not denied: %v", part.ID, settings)
		}
		env := settings["env"].(map[string]any)
		if settings["disableWorkflows"] != true || settings["idleCompaction"] != false || settings["disableClaudeAiConnectors"] != true || env["CLAUDE_CODE_DISABLE_CRON"] != "1" {
			t.Fatalf("%s: the off-list: %v", part.ID, settings)
		}
		dir := filepath.Join(home, ".config", "rewake", "claude", "worker")
		if adapter.HasFlag(plan.Args, mcpConfigFlag) || !slices.Contains(adapter.FlagValues(plan.Args, pluginDirFlag), dir) || slices.Contains(adapter.FlagValues(plan.Args, pluginDirFlag), filepath.Join(dir, "plugins")) {
			t.Fatalf("%s: the folder's parts: %q", part.ID, plan.Args)
		}
		if intro := adapter.FlagValues(plan.Args, introFlag); len(intro) != 1 || strings.HasSuffix(intro[0], "\n\n") {
			t.Fatalf("%s: an empty CLAUDE.md added to the briefing: %q", part.ID, intro)
		}
	}
}

// A worker's launch makes the service folder: private, a plugin of its
// own with an empty skills/, empty settings and instructions.
func TestTheServiceFolderIsMade(t *testing.T) {
	home := t.TempDir()
	if _, err := profileLaunch(t, home, role.Write, "", nil); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".config", "rewake", "claude", "worker")
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("the folder: %v %v", info, err)
	}
	if info, err := os.Stat(filepath.Join(dir, "skills")); err != nil || !info.IsDir() {
		t.Fatalf("skills/: %v", err)
	}
	var manifest map[string]any
	raw, _ := os.ReadFile(filepath.Join(dir, ".claude-plugin", "plugin.json"))
	if err := json.Unmarshal(raw, &manifest); err != nil || manifest["name"] != profilePlugin {
		t.Fatalf("plugin.json %q: %v", raw, err)
	}
	for name, want := range map[string]string{"settings.json": "{}\n", "CLAUDE.md": ""} {
		if raw, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(raw) != want {
			t.Fatalf("%s = %q, %v", name, raw, err)
		}
	}
}

// What the person put in the folder stays as it was.
func TestTheServiceFolderIsNeverOverwritten(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "rewake", "claude", "worker")
	if err := os.MkdirAll(filepath.Join(dir, ".claude-plugin"), 0o700); err != nil {
		t.Fatal(err)
	}
	own := map[string]string{
		"settings.json":              `{"language": "x"}`,
		"CLAUDE.md":                  "Mine.",
		".claude-plugin/plugin.json": `{"name": "mine"}`,
	}
	for name, text := range own {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(dir, "skills")); err != nil {
		t.Fatal(err)
	}
	if _, err := profileLaunch(t, home, role.General, "", nil); err != nil {
		t.Fatal(err)
	}
	for name, text := range own {
		if raw, _ := os.ReadFile(filepath.Join(dir, name)); string(raw) != text {
			t.Errorf("%s became %q", name, raw)
		}
	}
	if info, err := os.Lstat(filepath.Join(dir, "skills")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the linked skills/ was replaced: %v", err)
	}
}

// The person's denials add to rewake's; only rewake's own key gives a
// denied tool back, and it is not passed on.
func TestADeniedToolComesBackOnlyByRewakesKey(t *testing.T) {
	rest := func(drop ...string) []any {
		var out []any
		for _, tool := range deniedTools {
			if !slices.Contains(drop, tool) {
				out = append(out, tool)
			}
		}
		return out
	}
	for text, want := range map[string][]any{
		`{"permissions": {"deny": ["WebFetch"], "allow": ["Agent"]}}`:            append(rest(), "WebFetch"),
		`{"rewake": {"keep": ["Agent", "SendMessage"]}}`:                         rest("Agent", "SendMessage"),
		`{"rewake": {"keep": ["Agent"]}, "permissions": {"deny": ["WebFetch"]}}`: append(rest("Agent"), "WebFetch"),
		`{"rewake": {"keep": []}, "env": {"CLAUDE_CODE_DISABLE_CRON": "0"}}`:     rest(),
	} {
		home := t.TempDir()
		dir := filepath.Join(home, ".config", "rewake", "claude", "worker")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		plan, err := profileLaunch(t, home, role.Write, "", nil)
		if err != nil {
			t.Fatalf("%s: %v", text, err)
		}
		settings := settingsOf(t, plan.Args)
		deny := settings["permissions"].(map[string]any)["deny"].([]any)
		if _, passed := settings[rewakeKey]; passed || !slices.Equal(deny, want) {
			t.Errorf("%s: settings %v", text, settings)
		}
	}
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "rewake", "claude", "worker")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{`{"rewake": true}`, `{"rewake": {"subagents": true}}`, `{"rewake": {"keep": ["Bash"]}}`, `{"rewake": {"keep": "Agent"}}`, `{"permissions": {"deny": "Agent"}}`} {
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := profileLaunch(t, home, role.Write, "", nil); err == nil {
			t.Errorf("%s was accepted", text)
		}
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
