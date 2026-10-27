package claude

import (
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
	Refresh json.Number
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
	line := personalLine{Refresh: refreshOf(merged)}
	line.Type, _ = merged["type"].(string)
	line.Command, _ = merged["command"].(string)
	return line
}

// statusLineIn is the statusLine object of one settings file, nil where it
// has none or the file does not parse (the parser of the worker profile, so
// both readers take a file the same way).
func statusLineIn(path string) map[string]any {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	settings, err := parseSettings(data)
	if err != nil {
		return nil
	}
	line, _ := settings["statusLine"].(map[string]any)
	return line
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
