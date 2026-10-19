package claude

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/role"
)

// The worker profile (docs/v2/worker-profile.md): what a worker of Claude Code
// gets in place of the person's own global layer, passed as options of its one
// launch. Facts probed on 2.1.302 with `claude -p "/context"`, no model call.

// settingSourcesFlag with "project,local" leaves out the user layer: the
// person's settings.json, user skills, user MCP servers and user CLAUDE.md.
const settingSourcesFlag = "--setting-sources"

const workerSources = "project,local"

// settingsFlag takes a file or a JSON string. It is single-use — a second one
// replaces the first rather than merging — so the profile owns it.
const settingsFlag = "--settings"

const mcpConfigFlag = "--mcp-config"

// profileDirPath is the person's own part of the profile, beside rewake's
// alias file and pi's directory; rewake reads it and never writes it.
func profileDirPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("the worker profile is in the home directory: %w", err)
	}
	return filepath.Join(home, ".config", "rewake", "claude", "worker"), nil
}

// maxInstructions keeps the briefing with the profile's instructions within
// the 128 KiB Linux allows one argument.
const maxInstructions = 120 << 10

// profile is what the worker profile adds to one launch.
type profile struct {
	args         []string
	instructions string
	notes        []string
}

// workerProfile builds the profile for a launch: none for main, and none
// under --command, whose wrapper decides the configuration it starts with.
func workerProfile(request adapter.LaunchRequest, args []string) (profile, error) {
	if request.Role.ID == role.Main.ID {
		return profile{}, nil
	}
	if request.Command != "" {
		return profile{notes: []string{"the worker profile is not applied: the program named with --command decides the configuration Claude Code starts with"}}, nil
	}
	dir, err := profileDirPath()
	if err != nil {
		return profile{}, err
	}
	for _, flag := range []string{settingsFlag, settingSourcesFlag} {
		if adapter.HasFlag(args, flag) {
			return profile{}, fmt.Errorf("a worker's %s comes from rewake's worker profile; put the settings in %s", flag, filepath.Join(dir, "settings.json"))
		}
	}
	settings, err := profileSettings(dir, request.Env)
	if err != nil {
		return profile{}, err
	}
	out := profile{args: []string{settingSourcesFlag, workerSources, settingsFlag, settings}}
	plugins := filepath.Join(dir, "plugins")
	if ok, err := present(plugins, true); err != nil {
		return profile{}, err
	} else if ok {
		out.args = append(out.args, pluginDirFlag, plugins)
	}
	mcp := filepath.Join(dir, "mcp.json")
	if ok, err := present(mcp, false); err != nil {
		return profile{}, err
	} else if ok {
		out.args = append(out.args, mcpConfigFlag, mcp)
	}
	instructions := filepath.Join(dir, "CLAUDE.md")
	if ok, err := present(instructions, false); err != nil {
		return profile{}, err
	} else if ok {
		data, err := os.ReadFile(instructions)
		if err != nil {
			return profile{}, fmt.Errorf("the worker profile's %s cannot be read: %w", instructions, err)
		}
		if len(data) > maxInstructions {
			return profile{}, fmt.Errorf("the worker profile's %s is over %d KiB, more than one argument of a launch carries", instructions, maxInstructions>>10)
		}
		out.instructions = strings.TrimSpace(string(data))
	}
	return out, nil
}

// present says whether the profile has an entry of that kind; one of the
// other kind is a mistake said rather than skipped.
func present(path string, dir bool) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("the worker profile's %s cannot be read: %w", path, err)
	}
	if info.IsDir() != dir {
		want := "a file"
		if dir {
			want = "a directory"
		}
		return false, fmt.Errorf("the worker profile's %s is not %s", path, want)
	}
	return true, nil
}

// profileSettings is the one --settings value: rewake's defaults with the
// person's settings.json over them, as a JSON string, so nothing is written.
func profileSettings(dir string, env []string) (string, error) {
	settings := map[string]any{
		// A worker is short-lived; a project's auto-memory is main's, and a
		// shared directory would mix the two.
		"autoMemoryEnabled": false,
		// Claude Code reads .claude/CLAUDE.md of every directory above the
		// working one, so the person's own loads as a project's for any
		// checkout under the home directory, with the user source off too.
		"claudeMdExcludes": personalMemory(env),
		"env": map[string]any{
			// Not updated under a running room, nor a survey in a pane
			// nobody may watch.
			"DISABLE_AUTOUPDATER":      "1",
			"DISABLE_FEEDBACK_COMMAND": "1",
		},
	}
	path := filepath.Join(dir, "settings.json")
	if ok, err := present(path, false); err != nil {
		return "", err
	} else if ok {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("the worker profile's %s cannot be read: %w", path, err)
		}
		var own map[string]any
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if err := decoder.Decode(&own); err != nil || own == nil {
			return "", fmt.Errorf("the worker profile's %s is not a JSON object", path)
		}
		if err := mergeSettings(settings, own); err != nil {
			return "", fmt.Errorf("the worker profile's %s: %w", path, err)
		}
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// mergeSettings lays the person's settings over rewake's: objects key by key,
// the person's value winning, except claudeMdExcludes, which is added to, so
// the exclusion of the person's own CLAUDE.md is never lost.
func mergeSettings(base, own map[string]any) error {
	for key, value := range own {
		if key == "claudeMdExcludes" {
			list, ok := value.([]any)
			if !ok {
				return errors.New("claudeMdExcludes is not a list")
			}
			base[key] = append(base[key].([]any), list...)
			continue
		}
		mergeValue(base, key, value)
	}
	return nil
}

func mergeValue(base map[string]any, key string, value any) {
	inner, isObject := value.(map[string]any)
	current, wasObject := base[key].(map[string]any)
	if !isObject || !wasObject {
		base[key] = value
		return
	}
	for k, v := range inner {
		mergeValue(current, k, v)
	}
}

// personalMemory names the person's user CLAUDE.md: in Claude Code's
// configuration directory, and in ~/.claude when that directory is another.
func personalMemory(env []string) []any {
	home := envValue(env, "HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	var files []any
	if dir := envValue(env, "CLAUDE_CONFIG_DIR"); dir != "" {
		files = append(files, filepath.Join(dir, "CLAUDE.md"))
	}
	if home != "" {
		if own := filepath.Join(home, ".claude", "CLAUDE.md"); len(files) == 0 || files[0] != own {
			files = append(files, own)
		}
	}
	return files
}

// envValue reads one variable of the run's environment, the last one set.
func envValue(env []string, name string) string {
	value := ""
	for _, entry := range env {
		if k, v, ok := strings.Cut(entry, "="); ok && k == name {
			value = v
		}
	}
	return value
}
