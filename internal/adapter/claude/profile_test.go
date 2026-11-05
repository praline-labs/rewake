package claude

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
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

// profileDir is the service folder of a private home.
func profileDir(home string) string {
	return filepath.Join(home, ".config", "rewake", "claude", "worker")
}

// A worker runs in the service folder as its configuration directory, its
// settings.json holding rewake's defaults, with the run's own settings and
// no user layer left out: Claude Code reads the folder as the user's.
func TestWorkerProfileDefaults(t *testing.T) {
	home := t.TempDir()
	dir := profileDir(home)
	for _, part := range []role.Role{role.General, role.Write} {
		plan, err := profileLaunch(t, home, part, "", []string{"CLAUDE_CONFIG_DIR=/elsewhere"})
		if err != nil {
			t.Fatal(err)
		}
		if adapter.HasFlag(plan.Args, settingSourcesFlag) || !slices.Contains(plan.Env, configDirEnv+"="+dir) || slices.Contains(plan.Env, configDirEnv+"=/elsewhere") {
			t.Fatalf("%s: %q, env %q", part.ID, plan.Args, plan.Env)
		}
		if values := adapter.FlagValues(plan.Args, settingsFlag); len(values) != 1 || !strings.HasPrefix(values[0], "{") {
			t.Fatalf("%s: settings %q", part.ID, values)
		}
		settings := readBack(t, filepath.Join(dir, "settings.json"))
		excludes := []any{filepath.Join(home, ".claude", "CLAUDE.md")}
		if version, _ := settingsVersion(settings); version != defaultsVersion || !slices.Equal(settings["claudeMdExcludes"].([]any), excludes) {
			t.Fatalf("%s: settings %v", part.ID, settings)
		}
		if _, memory := settings["autoMemoryEnabled"]; memory {
			t.Fatalf("%s: settings %v", part.ID, settings)
		}
		if deny := denied(settings); len(deny) != len(deniedTools)-1 || deny[0] != "Agent" || slices.Contains(deny, any(enterTool)) {
			t.Fatalf("%s: the tools denied: %v", part.ID, deny)
		}
		env := settings["env"].(map[string]any)
		if settings["disableWorkflows"] != true || settings["idleCompaction"] != false || settings["disableClaudeAiConnectors"] != true || env["CLAUDE_CODE_DISABLE_CRON"] != "1" {
			t.Fatalf("%s: the off-list: %v", part.ID, settings)
		}
		if adapter.HasFlag(plan.Args, mcpConfigFlag) || slices.Contains(adapter.FlagValues(plan.Args, pluginDirFlag), dir) {
			t.Fatalf("%s: the folder's parts: %q", part.ID, plan.Args)
		}
	}
}

// A worker's launch makes the service folder: private, with empty
// instructions and no skills/, rewake's settings, and Claude Code's state
// saying onboarding is done and the project trusted.
func TestTheServiceFolderIsMade(t *testing.T) {
	home := t.TempDir()
	if _, err := profileLaunch(t, home, role.Write, "", nil); err != nil {
		t.Fatal(err)
	}
	dir := profileDir(home)
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("the folder: %v %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "skills")); !os.IsNotExist(err) {
		t.Fatalf("skills/ was made: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude-plugin")); err == nil {
		t.Fatal("the folder was made a plugin")
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md")); err != nil || len(raw) != 0 {
		t.Fatalf("CLAUDE.md = %q, %v", raw, err)
	}
	for _, name := range []string{"settings.json", claudeStateFile} {
		if info, err := os.Stat(filepath.Join(dir, name)); err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("%s: %v %v", name, info, err)
		}
	}
	cwd, _ := os.Getwd()
	saved := readBack(t, filepath.Join(dir, claudeStateFile))
	if saved["hasCompletedOnboarding"] != true || saved["projects"].(map[string]any)[projectRoot(cwd)].(map[string]any)["hasTrustDialogAccepted"] != true {
		t.Fatalf("Claude Code's state: %v", saved)
	}
}

// What the person put in the folder stays as it was; settings.json only
// gains rewake's defaults.
func TestTheServiceFolderIsNeverOverwritten(t *testing.T) {
	home := t.TempDir()
	dir := profileDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	own := map[string]string{"CLAUDE.md": "Mine."}
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
	if _, err := profileLaunch(t, home, role.General, "", nil); err != nil {
		t.Fatal(err)
	}
	if settings := readBack(t, filepath.Join(dir, "settings.json")); settings["language"] != "x" || settings["idleCompaction"] != false {
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

// A wrapper replaces the worker's program alone. Its launch gets the same
// service folder, run settings, checkout entry and inherited environment as a
// direct one, including the status line, memory, briefs and MCP file.
func TestWorkerProfileUnderCommand(t *testing.T) {
	base := t.TempDir()
	if err := os.Chmod(base, 0o700); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(base, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	dir := profileDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mcp.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	room := buildtest.Room(t, base, state.DefaultRoom)
	request := adapter.LaunchRequest{
		Name: "api", Dir: base, RoomDir: room, Intro: true, Role: role.Write, Template: "writer",
		RunDir: filepath.Join(room, "runs", "api.e1"), Executable: "/rewake", WorktreeEntry: true,
		Socket: filepath.Join(base, "api.sock"),
		Env:    []string{"HOME=" + home, "CLAUDE_CONFIG_DIR=/elsewhere", "WRAPPER_MARK=kept"},
	}
	plain, err := New().Launch(request)
	if err != nil {
		t.Fatal(err)
	}
	request.Command = "my-claude"
	wrapped, err := New().Launch(request)
	if err != nil {
		t.Fatal(err)
	}
	if wrapped.Command != "my-claude" || plain.Command != "claude" {
		t.Fatalf("programs: plain %q, wrapped %q", plain.Command, wrapped.Command)
	}
	if !slices.Equal(plain.Args, wrapped.Args) || !slices.Equal(plain.Env, wrapped.Env) {
		t.Fatalf("the launches differ:\nplain   %q %q\nwrapped %q %q", plain.Args, plain.Env, wrapped.Args, wrapped.Env)
	}
	settings := adapter.FlagValues(wrapped.Args, settingsFlag)
	if len(settings) != 1 || overlayOf(t, settings[0])[crossSessionSetting] != "accept" {
		t.Fatalf("run settings: %q", settings)
	}
	for _, want := range []string{configDirEnv + "=" + dir, "WRAPPER_MARK=kept", adapter.WorktreeEnterEnv + "=1"} {
		if !slices.Contains(wrapped.Env, want) {
			t.Errorf("environment lacks %q: %q", want, wrapped.Env)
		}
	}
	if slices.Contains(wrapped.Env, "CLAUDE_CONFIG_DIR=/elsewhere") {
		t.Errorf("the inherited configuration directory won: %q", wrapped.Env)
	}
	if values := adapter.FlagValues(wrapped.Args, mcpConfigFlag); !slices.Equal(values, []string{filepath.Join(dir, "mcp.json")}) {
		t.Errorf("MCP file: %q", values)
	}
	intro := adapter.FlagValues(wrapped.Args, introFlag)
	if len(intro) != 1 || !strings.Contains(intro[0], "\n"+entryBrief.Title+"\n") {
		t.Errorf("checkout briefing: %q", intro)
	}
}

// Main through a wrapper keeps the wrapper's settings and never gets the
// workers' service folder.
func TestMainProfileUnderCommand(t *testing.T) {
	home := t.TempDir()
	plan, err := profileLaunch(t, home, role.Main, "my-claude", nil, "--settings", "mine.json")
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(plan.Env, func(kv string) bool { return strings.HasPrefix(kv, configDirEnv+"=") }) || !slices.Equal(adapter.FlagValues(plan.Args, settingsFlag), []string{"mine.json"}) {
		t.Fatalf("the worker profile reached main: %q %q", plan.Args, plan.Env)
	}
	if _, err := os.Stat(profileDir(home)); !os.IsNotExist(err) {
		t.Fatalf("the worker folder was made: %v", err)
	}
}

func TestWorkerSettingsAreRefusedWithAndWithoutCommand(t *testing.T) {
	for _, command := range []string{"", "my-claude"} {
		for _, args := range [][]string{{settingsFlag, "{}"}, {settingsFlag + "={}"}, {settingSourcesFlag, "user"}, {settingSourcesFlag + "=user"}} {
			_, err := profileLaunch(t, t.TempDir(), role.Write, command, nil, args...)
			if err == nil || !strings.Contains(err.Error(), args[0]) && !strings.Contains(err.Error(), strings.SplitN(args[0], "=", 2)[0]) {
				t.Errorf("command %q, args %q: %v", command, args, err)
			}
		}
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

// A caller's own --append-system-prompt once replaced the briefing, since
// Claude Code reads only the last one: it joins it in the one rewake adds,
// and --no-intro drops only the briefing. The worker's CLAUDE.md is Claude
// Code's own user memory now, and stays out of it.
func TestTheCallersAppendedPromptJoinsTheBriefing(t *testing.T) {
	home := t.TempDir()
	dir := profileDir(home)
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
		{role.Write, true, []string{"--append-system-prompt", "MINE-MARK", "--", "--append-system-prompt"}, []string{`session "api"`, "MINE-MARK"}},
		{role.Write, false, []string{"--append-system-prompt", "MINE-MARK"}, []string{"MINE-MARK"}},
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
		if len(texts) != 1 || strings.Contains(texts[0], "FIRST-MARK") || strings.Contains(texts[0], "WORKER-MARK") {
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
