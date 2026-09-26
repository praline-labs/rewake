package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// The status line is one setting, and the layer rewake passes for a launch
// replaces the person's command with the tap (docs/research.md). So the tap
// runs the command the person would have had, found the way Claude Code finds
// it: every settings layer it reads, merged field by field, lowest first —
// user, project, local, then a --settings the caller passed. That order is
// Claude Code's own list in 2.1.280 (docs/research-launch.md).
//
// It is found each time the tap runs, from the tap's own environment and
// directory — which are the harness's. A person may set CLAUDE_CONFIG_DIR in a
// script of their own just before starting Claude Code, where the rewake
// wrapper never sees it; the tap does, because the harness starts it. Only
// what the harness cannot tell the tap is fixed at launch: which of the three
// files it reads, and the caller's --settings.
//
// The managed policy sits above all of these and above the tap too: a policy
// that names a status line wins, and the tap is then never run.
//
// Reading these files is reading, not editing: nothing here writes to them.

// Sources are the file layers a launch reads, as the tap receives them: a
// comma-separated subset of user, project and local, or "none".
const (
	AllSources = "user,project,local"
	NoSources  = "none"
)

// statusFields are the two fields that decide whether a status line runs and
// what. The others — padding, refreshInterval — reach the tap on their own,
// because the harness merges them over rewake's layer.
type statusFields struct {
	Kind    *string `json:"type"`
	Command *string `json:"command"`
}

// statusSettings is the one key of a settings file this reads.
type statusSettings struct {
	StatusLine json.RawMessage `json:"statusLine"`
}

// ResolveStatusCommand merges the layers and answers the command the session
// would have run as its status line, or "" when it would have run none. env
// and dir are the harness's, sources says which files the launch reads, and
// caller is the statusLine object of a --settings the caller passed, if any.
func ResolveStatusCommand(env []string, dir, sources string, caller json.RawMessage) string {
	enabled := map[string]bool{}
	for _, name := range strings.Split(sources, ",") {
		enabled[strings.TrimSpace(name)] = true
	}
	var merged statusFields
	for _, layer := range []struct{ name, path string }{
		{"user", userSettingsPath(env)},
		{"project", filepath.Join(dir, ".claude", "settings.json")},
		{"local", filepath.Join(dir, ".claude", "settings.local.json")},
	} {
		if enabled[layer.name] {
			mergeStatus(&merged, FileStatusLine(layer.path))
		}
	}
	mergeStatus(&merged, caller)
	if merged.Kind == nil || *merged.Kind != "command" || merged.Command == nil {
		return ""
	}
	return *merged.Command
}

// userSettingsPath is the user layer: the configuration directory the
// harness was given, or ~/.claude, and the alternative file name the harness
// switches to under its cowork variable.
func userSettingsPath(env []string) string {
	dir := lookup(env, "CLAUDE_CONFIG_DIR")
	if dir == "" {
		dir = filepath.Join(lookup(env, "HOME"), ".claude")
	}
	name := "settings.json"
	if value := lookup(env, "CLAUDE_CODE_USE_COWORK_PLUGINS"); value != "" && value != "0" && value != "false" {
		name = "cowork_settings.json"
	}
	return filepath.Join(dir, name)
}

// ProjectDir is where the harness reads project settings from: the directory
// it tells its commands about, or the one they run in.
func ProjectDir(env []string) string {
	if dir := lookup(env, "CLAUDE_PROJECT_DIR"); dir != "" {
		return dir
	}
	dir, _ := os.Getwd()
	return dir
}

func lookup(env []string, name string) string {
	for index := len(env) - 1; index >= 0; index-- {
		if key, value, ok := strings.Cut(env[index], "="); ok && key == name {
			return value
		}
	}
	return ""
}

// FileStatusLine is a file's statusLine object. A file that is missing or
// unreadable contributes nothing, as the harness skips one it cannot parse.
func FileStatusLine(path string) json.RawMessage {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var settings statusSettings
	if json.Unmarshal(raw, &settings) != nil {
		return nil
	}
	return settings.StatusLine
}

func mergeStatus(merged *statusFields, raw json.RawMessage) {
	if len(raw) == 0 {
		return
	}
	var layer statusFields
	if json.Unmarshal(raw, &layer) != nil {
		return
	}
	if layer.Kind != nil {
		merged.Kind = layer.Kind
	}
	if layer.Command != nil {
		merged.Command = layer.Command
	}
}
