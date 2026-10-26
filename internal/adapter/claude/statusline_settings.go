package claude

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// personalLine is the person's statusLine as far as rewake reads it: the
// command it runs, and the interval the run's own carries over.
type personalLine struct {
	Type    string
	Command string
	// Kept as read, json.Number for a number: a value of another type is
	// Claude Code's to judge, and must not lose the person the command.
	RefreshInterval any
}

// ownStatusLine is the statusLine the person's settings give a session in
// project — what Claude Code would draw without the run's own: the user's
// settings.json in configDir (else ~/.claude), the project's
// .claude/settings.json, then its .claude/settings.local.json, each later
// layer's keys over the earlier's, as Claude Code merges the key
// (docs/research.md, "How --settings meets a configured status line"). Only
// statusLine is read of each file; a file missing or unreadable adds nothing.
// A managed statusLine is not looked for: it outranks the run's own, which
// then never runs.
func ownStatusLine(configDir, project string) personalLine {
	if configDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			configDir = filepath.Join(home, ".claude")
		}
	}
	merged := map[string]any{}
	layers := []string{filepath.Join(configDir, "settings.json")}
	if project != "" {
		layers = append(layers, filepath.Join(project, ".claude", "settings.json"), localSettings(project))
	}
	for _, path := range layers {
		for key, value := range statusLineIn(path) {
			merged[key] = value
		}
	}
	line := personalLine{RefreshInterval: merged["refreshInterval"]}
	line.Type, _ = merged["type"].(string)
	line.Command, _ = merged["command"].(string)
	return line
}

// statusLineIn is the statusLine object of one settings file, nil where it
// has none.
func statusLineIn(path string) map[string]any {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var settings struct {
		StatusLine map[string]any `json:"statusLine"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if decoder.Decode(&settings) != nil {
		return nil
	}
	return settings.StatusLine
}

// localSettings is where Claude Code keeps a project's local settings: since
// 2.1.310 at the repository's canonical root — the main checkout's for a
// linked worktree — when the session starts below it, else in the session's
// own directory (read from the strings of 2.1.310, which also leaves the
// directory's own where the root is another user's).
func localSettings(project string) string {
	if root := canonicalRoot(project); root != "" {
		if path := filepath.Join(root, ".claude", "settings.local.json"); exists(path) {
			return path
		}
	}
	return filepath.Join(project, ".claude", "settings.local.json")
}

// canonicalRoot is the repository's root above dir: the directory holding
// .git, or for a linked worktree the main checkout its .git file points
// into; empty outside a repository.
func canonicalRoot(dir string) string {
	for at := dir; ; at = filepath.Dir(at) {
		entry := filepath.Join(at, ".git")
		info, err := os.Stat(entry)
		if err == nil && info.IsDir() {
			return at
		}
		if err == nil {
			data, err := os.ReadFile(entry)
			if err != nil {
				return at
			}
			gitdir := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(data)), "gitdir:"))
			if main, _, ok := strings.Cut(filepath.ToSlash(gitdir), "/.git/worktrees/"); ok && filepath.IsAbs(main) {
				return filepath.FromSlash(main)
			}
			return at
		}
		if filepath.Dir(at) == at {
			return ""
		}
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// projectDir is the session's project directory: the status line input's
// workspace.project_dir, else CLAUDE_PROJECT_DIR, which Claude Code sets for
// the command, else the directory the command runs in.
func projectDir(fromInput string) string {
	if fromInput != "" {
		return fromInput
	}
	if dir := os.Getenv("CLAUDE_PROJECT_DIR"); dir != "" {
		return dir
	}
	dir, _ := os.Getwd()
	return dir
}
