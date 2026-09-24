package workflow

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// launchLayerLike is a settings layer shaped like the one rewake builds: the
// end-of-turn hook in the foreground, the telemetry hook in the background on
// every event, and the status-line tap.
func launchLayerLike() string {
	type hook struct {
		Kind    string `json:"type"`
		Command string `json:"command"`
		Async   bool   `json:"async,omitempty"`
	}
	type matcher struct {
		Hooks []hook `json:"hooks"`
	}
	hooks := map[string][]matcher{"StopFailure": {{Hooks: []hook{{"command", "'/r' 'turn-ended'", false}}}}}
	for _, event := range telemetryEvents {
		hooks[event] = append(hooks[event], matcher{Hooks: []hook{{"command", "'/r' 'observe' '/s.obs'", true}}})
	}
	layer, _ := json.Marshal(map[string]any{
		"hooks":      hooks,
		"statusLine": map[string]string{"type": "command", "command": "'/r' 'status-tap' '/s.obs'"},
	})
	return string(layer)
}

// What the Claude Code fixture accepts as a launch, checked directly. The
// fixture must not be looser than the harness: a launch the real one refuses
// is a launch this one refuses too.
func TestClaudeShimRefusesWhatTheHarnessRefuses(t *testing.T) {
	good := []string{
		"--messaging-socket-path", "/tmp/s.sock",
		"--append-system-prompt", "brief",
		"--settings", launchLayerLike(),
		"--allowedTools", "Bash(rewake:*)",
	}
	if _, err := parseClaudeLaunch(good); err != nil {
		t.Fatalf("the launch rewake builds was refused: %v", err)
	}
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"a short flag the harness does not know", append([]string{"-m", "x"}, good...)},
		{"a long flag rewake does not pass", append([]string{"--verbose-debug", "x"}, good...)},
		{"a positional prompt", append(append([]string{}, good...), "hello")},
		{"a flag given twice", append(append([]string{}, good...), "--allowedTools", "Bash(x:*)")},
		{"a flag without its value", append(append([]string{}, good...), "--model")},
		{"settings without telemetry", []string{
			"--messaging-socket-path", "/tmp/s.sock", "--append-system-prompt", "brief",
			"--settings", `{"hooks":{"StopFailure":[{"hooks":[{"type":"command","command":"'/r' 'turn-ended'"}]}]}}`, "--allowedTools", "Bash(rewake:*)",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseClaudeLaunch(tc.args); err == nil {
				t.Errorf("accepted %v", tc.args)
			}
		})
	}
}

// A plugin directory the fixture accepts is one the harness would load: laid
// out as a plugin, with function hooks switched on. Anything else refuses the
// launch, so no case passes on a session that never heard its plugin.
func TestClaudeShimRefusesAPluginTheHarnessWouldNotLoad(t *testing.T) {
	t.Setenv(shimNoFunctionHooks, "")
	t.Setenv(shimNode, "/bin/true")
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv(functionHooksEnv, "1")
	if _, err := loadPlugin(dir); err == nil {
		t.Error("a directory with no manifest was accepted")
	}
	write(".claude-plugin/plugin.json", `{"name":"rewake"}`)
	if _, err := loadPlugin(dir); err == nil {
		t.Error("a plugin with no hooks file was accepted")
	}
	write("hooks/hooks.json", `{"modules":["./rewake.js"]}`)
	if _, err := loadPlugin(dir); err == nil {
		t.Error("a plugin naming a missing module was accepted")
	}
	write("hooks/rewake.js", "export function register() {}\n")
	if plugin, err := loadPlugin(dir); err != nil || plugin == nil {
		t.Errorf("a plugin laid out as one was refused: %v", err)
	}
	t.Setenv(functionHooksEnv, "")
	if _, err := loadPlugin(dir); err == nil {
		t.Error("a plugin without function hooks switched on was accepted")
	}
	if plugin, err := loadPlugin(""); err != nil || plugin != nil {
		t.Errorf("a launch without a plugin: %v, %v", plugin, err)
	}
}

// The fixture runs none of a plugin's handlers for an event that plugin's own
// call raised, as the harness does; an event from elsewhere reaches them.
func TestClaudeShimSkipsAPluginsHandlersForItsOwnCall(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; the module is not run")
	}
	dir := t.TempDir()
	module := filepath.Join(dir, "probe.js")
	source := `export function register(on) {
  on("session.compact", async ($, e, next) => {
    await $.fs.write(e.dir + "/" + e.trigger, "")
    return next(e)
  })
}
`
	if err := os.WriteFile(module, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	plugin := &claudePlugin{node: node, name: "rewake", modules: []string{module}}
	defer func() {
		for _, host := range plugin.hosts {
			if host != nil {
				_ = host.in.Close()
			}
		}
	}()
	plugin.raised(plugin.origin(), "session.compact", map[string]any{"trigger": "plugin", "dir": dir})
	plugin.raised("", "session.compact", map[string]any{"trigger": "manual", "dir": dir})
	if _, err := os.Stat(filepath.Join(dir, "plugin")); err == nil {
		t.Error("the plugin's handler ran for the compaction its own call raised")
	}
	if _, err := os.Stat(filepath.Join(dir, "manual")); err != nil {
		t.Errorf("the plugin's handler did not run for a compaction from elsewhere: %v", err)
	}
}
