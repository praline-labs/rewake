package claude

import (
	"maps"
	"path/filepath"
	"slices"

	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/state"
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

// launchSettings is the --settings a worker's launch passes: the folder's
// settings.json, or a copy in the run's directory with what this run alone
// gets laid over it — its template's memory, and the entry into a checkout —
// since --settings is single-use. It says whether the run may enter a
// checkout. With no run directory to write in, the worker starts without
// either, noted. A worker raised from no template — only a test build's
// direct launch — has none.
func launchSettings(path string, settings map[string]any, folder, runDir, template string, enter bool) (string, bool, []string, error) {
	if template == "" {
		return path, false, nil, nil
	}
	refused := func(why string, memory bool) []string {
		var notes []string
		if memory {
			notes = append(notes, "no worker memory: "+why)
		}
		if enter {
			notes = append(notes, "no worktree entry: "+why)
		}
		return notes
	}
	if runDir == "" {
		return path, false, refused("the launch has no run directory to write its settings in", true), nil
	}
	overlaid, memory, err := memorySettings(settings, folder, template)
	if err != nil {
		return "", false, nil, err
	}
	if enter {
		overlaid = withEntry(overlaid)
	}
	if !memory && !enter {
		return path, false, nil, nil
	}
	data, err := encodeSettings(overlaid)
	if err != nil {
		return "", false, nil, err
	}
	// A run whose directory cannot be written fails its mod at the start;
	// the memory and the entry alone stop nothing.
	run := filepath.Join(runDir, "worker-settings.json")
	err = state.EnsureSubdir(runDir)
	if err == nil {
		err = state.WriteFile(run, data, 0o600)
	}
	if err != nil {
		return path, false, refused("its settings could not be written to the run directory: "+err.Error(), memory), nil
	}
	return run, enter, nil, nil
}

// withEntry is the settings with EnterWorktree taken out of permissions.deny
// and allowed, nothing else changed: a whole-tool deny hides the tool from
// the mod's own call too (probed on 2.1.302). A deny the person wrote in
// another form, a rule with a specifier, stays and wins.
func withEntry(settings map[string]any) map[string]any {
	out := maps.Clone(settings)
	permissions, _ := out["permissions"].(map[string]any)
	permissions = maps.Clone(permissions)
	if permissions == nil {
		permissions = map[string]any{}
	}
	if deny, ok := permissions["deny"].([]any); ok {
		permissions["deny"] = slices.DeleteFunc(slices.Clone(deny), func(value any) bool { return value == enterTool })
	}
	allow, _ := permissions["allow"].([]any)
	if !slices.Contains(allow, any(enterTool)) {
		permissions["allow"] = append(slices.Clone(allow), enterTool)
	}
	out["permissions"] = permissions
	return out
}
