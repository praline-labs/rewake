package claude

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
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
// alias file and pi's directory; rewake makes what of it is missing and never
// changes what is there (makeProfileDir).
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
	if err := makeProfileDir(dir); err != nil {
		return profile{}, err
	}
	settings, err := profileSettings(dir, request.Env)
	if err != nil {
		return profile{}, err
	}
	// The directory is the worker plugin, its skills/ the person's skills.
	out := profile{args: []string{settingSourcesFlag, workerSources, settingsFlag, settings, pluginDirFlag, dir}}
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
			// nobody may watch, nor the person's IDE opened from one.
			"DISABLE_AUTOUPDATER":                 "1",
			"DISABLE_FEEDBACK_COMMAND":            "1",
			"CLAUDE_CODE_DISABLE_FEEDBACK_SURVEY": "1",
			"CLAUDE_CODE_AUTO_CONNECT_IDE":        "false",
			// A worker that wakes itself ends turns nobody asked for.
			"CLAUDE_CODE_DISABLE_CRON": "1",
		},
		"permissions": map[string]any{"deny": anyList(deniedTools)},
		// Fan-out, a session steered from outside the room, and model
		// calls and noise in a pane main reads.
		"disableWorkflows":        true,
		"remoteControlAtStartup":  false,
		"promptSuggestionEnabled": false,
		"awaySummaryEnabled":      false,
		"idleCompaction":          false,
		"spinnerTipsEnabled":      false,
		// The person's claude.ai connectors; the project's .mcp.json stays.
		"disableClaudeAiConnectors": true,
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
		keep, err := takeRewakeKey(own)
		if err != nil {
			return "", fmt.Errorf("the worker profile's %s: %w", path, err)
		}
		permissions := settings["permissions"].(map[string]any)
		permissions["deny"] = slices.DeleteFunc(permissions["deny"].([]any), func(tool any) bool { return slices.Contains(keep, tool.(string)) })
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

// deniedTools are the tools a worker goes without (the owner's decision,
// October 19, 2026; names of 2.1.302): subagents, every built-in agent
// among them, which run past main's view; waking itself; Claude Code's own
// messaging beside rewake's mail, whose socket still carries the wake; a
// checkout rewake does not know; plan mode, whose exit waits at a screen
// nobody watches; and what reaches outside the machine with no person there.
var deniedTools = []string{
	"Agent", "ScheduleWakeup", "SendMessage", "ListAgents", "EnterWorktree", "ExitWorktree",
	"EnterPlanMode", "PushNotification", "RemoteTrigger", "Artifact",
}

// anyList is a list of names as decoded JSON holds one.
func anyList(names []string) []any {
	out := make([]any, len(names))
	for i, name := range names {
		out[i] = name
	}
	return out
}

// rewakeKey is the profile settings' own object, read by rewake and not
// passed on. A deny cannot be undone by an allow, so the way back is here:
// {"rewake": {"keep": ["Agent"]}} gives workers back the tools it names.
const rewakeKey = "rewake"

// takeRewakeKey reads and removes rewake's own key from the person's
// settings, returning the denied tools workers keep.
func takeRewakeKey(own map[string]any) ([]string, error) {
	value, present := own[rewakeKey]
	if !present {
		return nil, nil
	}
	delete(own, rewakeKey)
	fields, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s is not an object", rewakeKey)
	}
	var keep []string
	for key, field := range fields {
		list, isList := field.([]any)
		if key != "keep" || !isList {
			return nil, fmt.Errorf("%s holds only \"keep\", a list of the tools rewake denies a worker, not %q", rewakeKey, key)
		}
		for _, item := range list {
			tool, isText := item.(string)
			if !isText || !slices.Contains(deniedTools, tool) {
				return nil, fmt.Errorf("%s.keep names %v, not one of the tools rewake denies a worker: %s", rewakeKey, item, strings.Join(deniedTools, ", "))
			}
			keep = append(keep, tool)
		}
	}
	return keep, nil
}

// mergeSettings lays the person's settings over rewake's: objects key by key,
// the person's value winning, except two lists, which are added to, since a
// list replaces rewake's whole: claudeMdExcludes, so the exclusion of the
// person's own CLAUDE.md is never lost, and permissions.deny, so the
// person's own denials do not lift rewake's.
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
		if permissions, ok := value.(map[string]any); ok && key == "permissions" {
			if denied, given := permissions["deny"]; given {
				list, ok := denied.([]any)
				if !ok {
					return errors.New("permissions.deny is not a list")
				}
				if current, ok := base[key].(map[string]any); ok {
					list = append(current["deny"].([]any), list...)
				}
				permissions = maps.Clone(permissions)
				permissions["deny"] = list
				value = permissions
			}
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
