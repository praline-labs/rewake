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

// settingsOf reads the one settings file the launch passes.
func settingsOf(t *testing.T, args []string) map[string]any {
	t.Helper()
	values := adapter.FlagValues(args, settingsFlag)
	if len(values) != 1 {
		t.Fatalf("want one %s, got %v in %q", settingsFlag, values, args)
	}
	raw, err := os.ReadFile(values[0])
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
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
		dir := filepath.Join(home, ".config", "rewake", "claude", "worker")
		if got := adapter.FlagValues(plan.Args, settingsFlag); !slices.Equal(got, []string{filepath.Join(dir, "settings.json")}) {
			t.Fatalf("%s: settings %v", part.ID, got)
		}
		settings := settingsOf(t, plan.Args)
		excludes := []any{filepath.Join(home, ".claude", "CLAUDE.md")}
		if _, memory := settings["autoMemoryEnabled"]; memory || !slices.Equal(settings["claudeMdExcludes"].([]any), excludes) || settings[rewakeKey].(map[string]any)["defaults"] != float64(defaultsVersion) {
			t.Fatalf("%s: settings %v", part.ID, settings)
		}
		if deny := settings["permissions"].(map[string]any)["deny"].([]any); len(deny) != len(deniedTools) || deny[0] != "Agent" {
			t.Fatalf("%s: the tools not denied: %v", part.ID, settings)
		}
		env := settings["env"].(map[string]any)
		if settings["disableWorkflows"] != true || settings["idleCompaction"] != false || settings["disableClaudeAiConnectors"] != true || env["CLAUDE_CODE_DISABLE_CRON"] != "1" {
			t.Fatalf("%s: the off-list: %v", part.ID, settings)
		}
		if adapter.HasFlag(plan.Args, mcpConfigFlag) || !slices.Contains(adapter.FlagValues(plan.Args, pluginDirFlag), dir) || slices.Contains(adapter.FlagValues(plan.Args, pluginDirFlag), filepath.Join(dir, "plugins")) {
			t.Fatalf("%s: the folder's parts: %q", part.ID, plan.Args)
		}
		if intro := adapter.FlagValues(plan.Args, introFlag); len(intro) != 1 || strings.HasSuffix(intro[0], "\n\n") {
			t.Fatalf("%s: an empty CLAUDE.md added to the briefing: %q", part.ID, intro)
		}
	}
}

// A worker's launch makes the service folder: private, a plugin of its
// own with an empty skills/ and instructions, and rewake's settings.
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
	if raw, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md")); err != nil || len(raw) != 0 {
		t.Fatalf("CLAUDE.md = %q, %v", raw, err)
	}
	if info, err := os.Stat(filepath.Join(dir, "settings.json")); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("settings.json: %v %v", info, err)
	}
}

// What the person put in the folder stays as it was; settings.json only
// gains rewake's defaults.
func TestTheServiceFolderIsNeverOverwritten(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "rewake", "claude", "worker")
	if err := os.MkdirAll(filepath.Join(dir, ".claude-plugin"), 0o700); err != nil {
		t.Fatal(err)
	}
	own := map[string]string{
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
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"language": "x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := profileLaunch(t, home, role.General, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if settings := settingsOf(t, plan.Args); settings["language"] != "x" || settings["idleCompaction"] != false {
		t.Errorf("settings.json became %v", settings)
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

// A caller's own --append-system-prompt once replaced the briefing and the
// worker's CLAUDE.md, since Claude Code reads only the last one: it joins
// them in the one rewake adds, and --no-intro drops only the briefing.
func TestTheCallersAppendedPromptJoinsTheBriefing(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "rewake", "claude", "worker")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("WORKER-MARK"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		part  role.Role
		intro bool
		args  []string
		want  []string
	}{
		{role.Main, true, []string{"--append-system-prompt", "FIRST-MARK", "--append-system-prompt=MINE-MARK"}, []string{`session "api"`, "MINE-MARK"}},
		{role.Write, true, []string{"--append-system-prompt", "MINE-MARK", "--", "--append-system-prompt"}, []string{`session "api"`, "WORKER-MARK", "MINE-MARK"}},
		{role.Write, false, []string{"--append-system-prompt", "MINE-MARK"}, []string{"WORKER-MARK", "MINE-MARK"}},
	} {
		t.Setenv("HOME", home)
		plan, err := New().Launch(adapter.LaunchRequest{
			Name: "api", Dir: t.TempDir(), Args: c.args, Intro: c.intro, Role: c.part,
			Socket: filepath.Join(t.TempDir(), "api.sock"), Env: []string{"HOME=" + home},
		})
		if err != nil {
			t.Fatal(err)
		}
		texts := adapter.FlagValues(plan.Args, introFlag)
		if len(texts) != 1 || strings.Contains(texts[0], "FIRST-MARK") {
			t.Fatalf("%v: %q", c.args, texts)
		}
		at := -1
		for _, part := range c.want {
			next := strings.Index(texts[0], part)
			if next <= at {
				t.Fatalf("%v: %q not after what precedes it in %q", c.args, part, texts[0])
			}
			at = next
		}
		if !c.intro && strings.Contains(texts[0], `session "api"`) {
			t.Fatalf("--no-intro kept the briefing: %q", texts[0])
		}
		if c.args[len(c.args)-1] == "--append-system-prompt" && plan.Args[len(plan.Args)-1] != "--append-system-prompt" {
			t.Fatalf("the prompt after -- was touched: %q", plan.Args)
		}
	}
}
