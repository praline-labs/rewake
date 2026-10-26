package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/role"
)

// entryBrief is what a worker that may enter a checkout is told at its start:
// it starts in main's own checkout, and nothing else says so. A worker of
// such a template, never told, ran git switch -c in main's checkout and
// main's merge went to the wrong branch (October 21, 2026); the briefing is
// what the model reads every time, the tool's text only when it looks at it.
var entryBrief = role.Section{
	Title: "YOUR CHECKOUT",
	Lines: []string{
		"You start in main's own checkout, which main and other sessions work in. Do not change it: no edits, new or deleted files, commits, branch switches or creations, stashes or resets there.",
		"Before your first change to any file of the repository, call the tool worktree_enter: it makes a worktree of your own on a new branch from the commit you are at and moves this session into it; work and commit there. A task that only reads needs none.",
		"If worktree_enter fails, do not fall back to main's checkout: tell main with --notify what failed.",
	},
}

// enterTool is Claude Code's tool that moves a session into a checkout; a
// worker of a template with enter_worktree = true gets it back for rewake's
// worktree_enter to call (docs/worktree-tools.md), and the mod's guard
// refuses the model's own call.
const enterTool = "EnterWorktree"

// runSettings is the one --settings a worker's launch passes, as a JSON
// object: what this run alone gets over the folder's settings.json, which
// Claude Code reads itself as the user's settings — its memory, under the
// key memoryKey chose; EnterWorktree, denied unless the run may enter a
// checkout; and a read of main's checkout (readRule). Flag settings merge
// with the user's (probed on 2.1.302), but a deny there is not lifted by an
// allow here, so the folder's file no longer
// holds EnterWorktree (defaults version 3). It says whether the run may
// enter. A worker raised from no template — only a test build's direct
// launch — has no memory.
func runSettings(settings map[string]any, folder, root, memoryKey string, enter bool, runDir string) (string, bool, []string, error) {
	overlay := map[string]any{}
	if memoryKey != "" && settings["autoMemoryEnabled"] != false {
		dir, err := memoryDir(folder, root, memoryKey)
		if err != nil {
			return "", false, nil, err
		}
		if err := makeMemoryDir(dir); err != nil {
			return "", false, nil, err
		}
		overlay["autoMemoryEnabled"] = true
		overlay["autoMemoryDirectory"] = dir
	}
	// The status line is where Claude Code tells the cache's lifetime
	// (statusline.go); it needs the run's host, which a launch with no run
	// directory has no mod for.
	if executable, err := os.Executable(); err == nil && runDir != "" {
		line, _ := settings["statusLine"].(map[string]any)
		overlay["statusLine"] = statusLineSetting(executable, line["refreshInterval"])
	}
	var notes []string
	switch {
	case !enter:
	case runDir == "":
		// worktree_enter is a tool of the run's mod, which loads from the
		// run's directory.
		notes, enter = append(notes, "no worktree entry: the launch has no run directory for rewake's mod"), false
	case slices.Contains(deniedIn(settings), any(enterTool)):
		notes, enter = append(notes, "no worktree entry: the worker settings deny "+enterTool+"; take it out of permissions.deny there"), false
	}
	permissions := map[string]any{}
	allow := []any{readRule(root)}
	if enter {
		allow = append(allow, enterTool)
	} else {
		permissions["deny"] = []any{enterTool}
	}
	permissions["allow"] = allow
	overlay["permissions"] = permissions
	data, err := json.Marshal(overlay)
	if err != nil {
		return "", false, nil, err
	}
	return string(data), enter, notes, nil
}

// readRule lets the run read main's checkout, the repository's root every
// worktree of it shares, where main keeps the briefs and reviews a worker
// reads: a worker in a checkout of its own otherwise met auto mode's
// one-time prompt for a read outside its working directories, with nobody at
// its screen (October 23, 2026). A Read rule covers Read, Grep and Glob and
// grants no edit, which --add-dir would (docs/launch.md). The leading slash
// makes the rule's path absolute, "//home/...".
func readRule(root string) string {
	return "Read(/" + filepath.ToSlash(root) + "/**)"
}

// deniedIn is the settings' permissions.deny, none where it is missing.
func deniedIn(settings map[string]any) []any {
	permissions, _ := settings["permissions"].(map[string]any)
	deny, _ := permissions["deny"].([]any)
	return deny
}

// statusLineSetting is the run's statusLine: rewake's command, which runs the
// person's own (statusline.go), so it keeps that line's refreshInterval — how
// often the command runs beside the harness's events — or rewake's own where
// the line has none. Claude Code merges the key field by field, so the
// person's padding stands (docs/research.md, "How --settings meets a
// configured status line").
func statusLineSetting(executable string, every any) map[string]any {
	line := map[string]any{
		"type":            "command",
		"command":         shellQuote(executable) + " statusline",
		"refreshInterval": statusLineRefresh,
	}
	// The settings are read with numbers kept as json.Number.
	if every, ok := every.(json.Number); ok {
		if seconds, err := every.Float64(); err == nil && seconds >= 1 {
			line["refreshInterval"] = every
		}
	}
	return line
}

// mainSettings is the one --settings main's launch passes: the status line
// alone, so the host hears the model's name, its thinking, the cache's life
// and a changed effort as a worker's does (docs/claude.md#the-cache-ttl). It
// reads only the statusLine of the person's own settings (ownStatusLine), for
// its interval. None where the
// launch has no run directory for the mod, under --command, whose wrapper
// decides the configuration, where the person passes a --settings of their
// own, which a second would replace, or a --setting-sources, whose layers
// rewake's line would not keep to when it finds the person's.
func mainSettings(request adapter.LaunchRequest, args []string) profile {
	switch {
	case request.RunDir == "", request.Command != "":
		return profile{}
	}
	for _, flag := range []string{settingsFlag, settingSourcesFlag} {
		if adapter.HasFlag(args, flag) {
			return profile{notes: []string{"no status line of rewake's: the launch passes its own " + flag + ", so main's row shows the model's id and no cache life"}}
		}
	}
	executable, err := os.Executable()
	if err != nil {
		return profile{}
	}
	cwd, _ := os.Getwd()
	own := ownStatusLine(os.Getenv(configDirEnv), cwd)
	data, err := json.Marshal(map[string]any{"statusLine": statusLineSetting(executable, own.RefreshInterval)})
	if err != nil {
		return profile{}
	}
	return profile{args: []string{settingsFlag, string(data)}}
}

// shellQuote makes a path one word of the status line's shell command.
func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}
