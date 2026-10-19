package claude

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
)

// The worker's settings (the owner's decision, October 19, 2026): the service
// folder's settings.json is the whole of a worker's --settings, passed as the
// file it is. rewake writes its defaults there on the first launch, and when a
// later build brings new ones it adds only those, so what the person took out
// of an older version stays out.

// defaultsVersion is the version of rewake's defaults this build brings. A
// default added or retired raises it and names it in since or retired.
const defaultsVersion = 1

// rewakeKey is rewake's own object in the file, {"defaults": <version>}, the
// version the file is at. Claude Code ignores a key it does not know (probed
// on 2.1.302).
const rewakeKey = "rewake"

// setting is one of rewake's defaults: a key with its value, or, with item, a
// value of the list at path. Version since brings it; version retired, once
// set, takes it out where it still holds rewake's value.
type setting struct {
	path    []string
	value   any
	item    bool
	since   int
	retired int
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

// workerDefaults are rewake's defaults for the folder of that home.
func workerDefaults(home string) []setting {
	key := func(value any, path ...string) setting { return setting{path: path, value: value, since: 1} }
	item := func(value any, path ...string) setting {
		return setting{path: path, value: value, item: true, since: 1}
	}
	defaults := []setting{
		// A worker is short-lived; a project's auto-memory is main's, and a
		// shared directory would mix the two.
		key(false, "autoMemoryEnabled"),
		// Claude Code reads .claude/CLAUDE.md of every directory above the
		// working one, so the person's own loads as a project's for any
		// checkout under the home directory, with the user source off too.
		// A CLAUDE_CONFIG_DIR elsewhere is not read with the user source off.
		item(filepath.Join(home, ".claude", "CLAUDE.md"), "claudeMdExcludes"),
		// Not updated under a running room, nor a survey in a pane nobody
		// may watch, nor the person's IDE opened from one.
		key("1", "env", "DISABLE_AUTOUPDATER"),
		key("1", "env", "DISABLE_FEEDBACK_COMMAND"),
		key("1", "env", "CLAUDE_CODE_DISABLE_FEEDBACK_SURVEY"),
		key("false", "env", "CLAUDE_CODE_AUTO_CONNECT_IDE"),
		// A worker that wakes itself ends turns nobody asked for.
		key("1", "env", "CLAUDE_CODE_DISABLE_CRON"),
	}
	for _, tool := range deniedTools {
		defaults = append(defaults, item(tool, "permissions", "deny"))
	}
	return append(defaults,
		// Fan-out, a session steered from outside the room, and model calls
		// and noise in a pane main reads.
		key(true, "disableWorkflows"),
		key(false, "remoteControlAtStartup"),
		key(false, "promptSuggestionEnabled"),
		key(false, "awaySummaryEnabled"),
		key(false, "idleCompaction"),
		key(false, "spinnerTipsEnabled"),
		// The person's claude.ai connectors; the project's .mcp.json stays.
		key(true, "disableClaudeAiConnectors"),
	)
}

// workerSettings brings the folder's settings.json up to this build's
// defaults and returns its path; a file with none is at version 0. It says
// whether it wrote, so the launch can tell the person.
func workerSettings(dir, home string) (string, bool, error) {
	path := filepath.Join(dir, "settings.json")
	if _, err := present(path, false); err != nil {
		return "", false, err
	}
	settings, version, err := readSettings(path)
	if err != nil {
		return "", false, err
	}
	if version >= defaultsVersion {
		return path, false, nil
	}
	if err := migrate(settings, version, defaultsVersion, workerDefaults(home)); err != nil {
		return "", false, fmt.Errorf("the worker profile's %s: %w", path, err)
	}
	if err := writeSettings(path, settings); err != nil {
		return "", false, fmt.Errorf("the worker profile's %s could not be written: %w", path, err)
	}
	return path, true, nil
}

// readSettings reads the file and the defaults version it is at; a missing
// file is an empty one. One that does not parse is refused, never replaced.
func readSettings(path string) (map[string]any, int, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]any{}, 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("the worker profile's %s cannot be read: %w", path, err)
	}
	settings, err := parseSettings(data)
	if err != nil {
		return nil, 0, fmt.Errorf("the worker profile's %s, %w; rewake does not replace a file it cannot read", path, err)
	}
	version, err := settingsVersion(settings)
	if err != nil {
		return nil, 0, fmt.Errorf("the worker profile's %s: %w", path, err)
	}
	return settings, version, nil
}

// parseSettings decodes one JSON object, numbers kept as written, and names
// the line of what stops it.
func parseSettings(data []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var settings map[string]any
	err := decoder.Decode(&settings)
	if err == nil {
		if _, next := decoder.Token(); next != io.EOF {
			return nil, fmt.Errorf("line %d: more after the object", lineAt(data, decoder.InputOffset()))
		}
		if settings == nil {
			return nil, errors.New("line 1: not a JSON object")
		}
		return settings, nil
	}
	var syntax *json.SyntaxError
	var typed *json.UnmarshalTypeError
	switch {
	case errors.As(err, &syntax):
		return nil, fmt.Errorf("line %d: %s", lineAt(data, syntax.Offset), syntax.Error())
	case errors.As(err, &typed):
		return nil, fmt.Errorf("line %d: not a JSON object", lineAt(data, typed.Offset))
	case errors.Is(err, io.EOF):
		return nil, errors.New("line 1: empty, not a JSON object")
	default:
		return nil, fmt.Errorf("line %d: %w", lineAt(data, int64(len(data))), err)
	}
}

func lineAt(data []byte, offset int64) int {
	return bytes.Count(data[:min(max(offset, 0), int64(len(data)))], []byte("\n")) + 1
}

// settingsVersion reads rewake's key: absent is version 0.
func settingsVersion(settings map[string]any) (int, error) {
	value, ok := settings[rewakeKey]
	if !ok {
		return 0, nil
	}
	fields, ok := value.(map[string]any)
	if !ok {
		return 0, fmt.Errorf("%s is not an object", rewakeKey)
	}
	for field := range fields {
		switch field {
		case "defaults":
		case "keep":
			return 0, fmt.Errorf("%s.keep is gone: a denied tool comes back by taking it out of permissions.deny in this file", rewakeKey)
		default:
			return 0, fmt.Errorf("%s holds only \"defaults\", the version of rewake's defaults the file is at, not %q", rewakeKey, field)
		}
	}
	number, ok := fields["defaults"].(json.Number)
	version, err := number.Int64()
	if !ok || err != nil || version < 0 {
		return 0, fmt.Errorf("%s.defaults is not a version, a whole number from 0", rewakeKey)
	}
	return int(min(version, 1<<31)), nil
}

// migrate brings settings at version from to version to, running in order
// the steps of each version between: a default it brings is added where the
// file has no value for it, and one it retires is taken out where the file
// still holds rewake's value. A default of an older version is never run
// again, so the person's removal of it holds.
func migrate(settings map[string]any, from, to int, defaults []setting) error {
	for version := from + 1; version <= to; version++ {
		for _, one := range defaults {
			var err error
			switch version {
			case one.since:
				err = one.add(settings)
			case one.retired:
				err = one.remove(settings)
			}
			if err != nil {
				return err
			}
		}
	}
	settings[rewakeKey] = map[string]any{"defaults": to}
	return nil
}

func (s setting) add(settings map[string]any) error {
	parent, err := objectAt(settings, s.path, true)
	if err != nil {
		return err
	}
	last := s.path[len(s.path)-1]
	current, has := parent[last]
	switch {
	case !has && s.item:
		parent[last] = []any{s.value}
	case !has:
		parent[last] = s.value
	case s.item:
		list, ok := current.([]any)
		if !ok {
			return fmt.Errorf("%s is not a list", strings.Join(s.path, "."))
		}
		if !slices.Contains(list, s.value) {
			parent[last] = append(list, s.value)
		}
	}
	return nil
}

func (s setting) remove(settings map[string]any) error {
	parent, err := objectAt(settings, s.path, false)
	if parent == nil || err != nil {
		return err
	}
	last := s.path[len(s.path)-1]
	current, has := parent[last]
	switch {
	case !has:
	case s.item:
		list, ok := current.([]any)
		if !ok {
			return fmt.Errorf("%s is not a list", strings.Join(s.path, "."))
		}
		parent[last] = slices.DeleteFunc(list, func(value any) bool { return value == s.value })
	case reflect.DeepEqual(current, s.value):
		delete(parent, last)
	}
	return nil
}

// objectAt is the object holding the last key of path, made where missing
// when create is set, nil where missing otherwise.
func objectAt(settings map[string]any, path []string, create bool) (map[string]any, error) {
	object := settings
	for i, key := range path[:len(path)-1] {
		value, has := object[key]
		if !has {
			if !create {
				return nil, nil
			}
			value = map[string]any{}
			object[key] = value
		}
		inner, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s is not an object", strings.Join(path[:i+1], "."))
		}
		object = inner
	}
	return object, nil
}

// writeSettings writes the file as the person reads it: indented, keys in
// order, nothing escaped that JSON does not need. Through a symlink it
// replaces the file linked to, so a link into the person's own files stays.
func writeSettings(path string, settings map[string]any) error {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(settings); err != nil {
		return err
	}
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	return replaceFile(path, out.Bytes())
}
