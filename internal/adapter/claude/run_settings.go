package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

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
		overlay["statusLine"] = map[string]any{
			"type":            "command",
			"command":         shellQuote(executable) + " statusline",
			"refreshInterval": statusLineInterval(settings),
		}
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

// statusLineInterval is how often the run's status line command runs beside
// the harness's events: the person's own interval where the folder's settings
// have a status line with one, since that line is run by ours.
func statusLineInterval(settings map[string]any) any {
	line, _ := settings["statusLine"].(map[string]any)
	// The folder's settings are read with numbers kept as json.Number.
	if every, ok := line["refreshInterval"].(json.Number); ok {
		if seconds, err := every.Float64(); err == nil && seconds >= 1 {
			return every
		}
	}
	return statusLineRefresh
}

// shellQuote makes a path one word of the status line's shell command.
func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}
