package claude

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/harness/claude/telemetry"
)

// The function-hooks plugin: how rewake hears a turn a person interrupted.
//
// Claude Code runs no hook when a person presses Esc or Ctrl+C, and the wrapper
// gets no signal either (docs/research-claude-control.md). A function-hooks
// plugin does hear it: turn.complete with reason "aborted". So rewake writes a
// plugin of its own for each launch, into its state directory, and passes it
// for that launch only — a flag and an environment variable; rewake writes
// nothing into the person's configuration, though the switch reaches further
// than this plugin (docs/claude-plugin.md). The plugin only observes: it
// reports turn starts, turn ends and the context fill through `rewake
// observe`, the command the telemetry hooks run, and the wrapper decides what
// they mean.
//
// The API is an early-access one and may change between releases; when the
// plugin does not load — another version, an untrusted workspace, --bare,
// disableAllHooks, a module that fails — the session runs exactly as without
// it, and its telemetry says interruptions are unobserved.

//go:embed plugin.js
var pluginModule string

const (
	// functionHooksEnv switches function hooks on for the launch; they are
	// off by default on 2.1.280.
	functionHooksEnv = "CLAUDE_CODE_ENABLE_FUNCTION_HOOKS"
	pluginDirFlag    = "--plugin-dir"
	// bareFlag loads no plugin at all.
	bareFlag = "--bare"
	// pluginArgv is the placeholder the module's command replaces.
	pluginArgv = "__REWAKE_ARGV__"
)

// applyPlugin writes the plugin for this launch and passes it. What it could
// not do comes back as a note; the launch goes on either way, as it did before
// the plugin existed.
func applyPlugin(args, env []string, socket string) ([]string, []string, []string) {
	if socket == "" {
		return args, env, nil
	}
	const unheard = "not hearing interrupted turns: "
	if harness.HasFlag(args, bareFlag) {
		return args, env, []string{unheard + bareFlag + " loads no plugins"}
	}
	if value, set := lookupEnv(env, functionHooksEnv); set && !enabled(value) {
		// The person switched function hooks off for this launch; turning
		// them back on would change more than rewake's own plugin.
		return args, env, []string{unheard + functionHooksEnv + "=" + value + " keeps function hooks off"}
	}
	executable, err := os.Executable()
	if err != nil {
		return args, env, []string{unheard + "could not find the rewake binary: " + err.Error()}
	}
	dir := telemetry.PluginPath(socket)
	if err := writePlugin(dir, []string{executable, harness.Observe, socket}); err != nil {
		return args, env, []string{unheard + "could not write the plugin: " + err.Error()}
	}
	return harness.AddFlags(args, pluginDirFlag, dir), setEnv(env, functionHooksEnv, "1"), nil
}

// writePlugin lays out a plugin directory: its manifest, the hooks file naming
// the module, and the module with the command it runs written in.
func writePlugin(dir string, argv []string) error {
	encoded, err := json.Marshal(argv)
	if err != nil {
		return err
	}
	if !strings.Contains(pluginModule, pluginArgv) {
		return fmt.Errorf("the plugin module has no %s", pluginArgv)
	}
	// A leftover from a run that died under the same name holds nothing of
	// this one.
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	files := map[string]string{
		filepath.Join(".claude-plugin", "plugin.json"): `{"name":"rewake","version":"1.0.0","description":"Tells the rewake wrapper when a turn starts and ends, and how full the context is."}` + "\n",
		filepath.Join("hooks", "hooks.json"):           `{"modules":["./rewake.js"]}` + "\n",
		filepath.Join("hooks", "rewake.js"):            strings.Replace(pluginModule, pluginArgv, string(encoded), 1),
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func lookupEnv(env []string, name string) (string, bool) {
	for index := len(env) - 1; index >= 0; index-- {
		if value, found := strings.CutPrefix(env[index], name+"="); found {
			return value, true
		}
	}
	return "", false
}

func setEnv(env []string, name, value string) []string {
	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, name+"=") {
			out = append(out, entry)
		}
	}
	return append(out, name+"="+value)
}

// enabled reads a switch the way such variables are usually read: anything
// but an empty value, 0, false, no or off is on.
func enabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "0", "false", "no", "off":
		return false
	}
	return true
}
