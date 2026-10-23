package claude

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// The folder's .claude.json is Claude Code's own state: onboarding, folder
// trust, user MCP servers, counters. A fresh one stops an interactive start
// on the theme picker, then on the folder-trust dialog, with nobody at the
// worker's screen (probed on 2.1.302). Before each worker's launch rewake adds
// what of the two is missing and changes nothing else (the owner's decision,
// October 23, 2026): the trust of the repository main runs in, which the
// person's own sessions there already have, mirrored without reading the
// person's file.
//
// Claude Code reads the file again before it saves (probed: a running
// session's save at its exit kept an entry written meanwhile), so a write
// replacing it in one step is enough. Known limit: a save between rewake's
// read and its rename drops the entry, the worker then shows the dialog in
// its pane, and the next launch adds it again.

// claudeStateFile is the state file's name in the configuration directory.
const claudeStateFile = ".claude.json"

// prepareClaudeState makes sure the folder's state file says onboarding is
// done and the project at root is trusted. Claude Code keeps trust by the
// repository's main checkout and a trusted one covers its worktrees and
// subdirectories; a trusted parent does not cover a repository (probed on
// 2.1.302).
func prepareClaudeState(dir, root string) error {
	path := filepath.Join(dir, claudeStateFile)
	target, err := settingsTarget(path)
	if err != nil {
		return err
	}
	saved := map[string]any{}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return fmt.Errorf("the workers' Claude Code state %s cannot be read: %w", path, err)
	default:
		if saved, err = parseSettings(data); err != nil {
			return fmt.Errorf("the workers' Claude Code state %s, %w; rewake does not replace a file it cannot read", path, err)
		}
	}
	changed, err := markReady(saved, root)
	if err != nil {
		return fmt.Errorf("the workers' Claude Code state %s: %w", path, err)
	}
	if !changed {
		return nil
	}
	if err := writeSettings(target, saved); err != nil {
		return fmt.Errorf("the workers' Claude Code state %s could not be written: %w", path, err)
	}
	return nil
}

// markReady adds onboarding done and the trust of root, under its path as
// given and resolved, and says whether anything changed.
func markReady(saved map[string]any, root string) (bool, error) {
	changed := false
	if saved["hasCompletedOnboarding"] != true {
		saved["hasCompletedOnboarding"] = true
		changed = true
	}
	projects := map[string]any{}
	if value, has := saved["projects"]; has {
		object, ok := value.(map[string]any)
		if !ok {
			return false, errors.New("projects is not an object")
		}
		projects = object
	}
	keys := []string{root}
	if resolved, err := filepath.EvalSymlinks(root); err == nil && resolved != root {
		keys = append(keys, resolved)
	}
	for _, key := range keys {
		project := map[string]any{}
		if value, has := projects[key]; has {
			object, ok := value.(map[string]any)
			if !ok {
				return false, fmt.Errorf("projects[%q] is not an object", key)
			}
			project = object
		}
		if project["hasTrustDialogAccepted"] != true {
			project["hasTrustDialogAccepted"] = true
			projects[key] = project
			changed = true
		}
	}
	saved["projects"] = projects
	return changed, nil
}
