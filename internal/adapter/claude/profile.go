package claude

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/role"
)

// The worker profile (docs/v2/worker-profile.md): what a worker of Claude Code
// gets in place of the person's own global layer. Since October 23, 2026 (the
// owner's decision) it is a configuration directory of its own, the service
// folder, whose settings.json, CLAUDE.md and skills/ Claude Code reads as the
// user's, and whose login, trust and transcripts are the workers' alone; the
// launch adds what one run alone gets. Facts probed on 2.1.302 with
// `claude -p "/context"`, no model call.

// settingSourcesFlag names the setting layers a run reads. A worker's user
// layer is rewake's folder, so a worker's own is refused: one leaving out
// the user layer would drop rewake's defaults with it.
const settingSourcesFlag = "--setting-sources"

// settingsFlag takes a file or a JSON string. It is single-use — a second one
// replaces the first rather than merging — so the profile owns it, and passes
// what the run alone gets (runSettings).
const settingsFlag = "--settings"

const mcpConfigFlag = "--mcp-config"

// profileDirPath is the person's own part of the profile, beside rewake's
// alias file and pi's directory, and the home it lies in; rewake makes what of
// it is missing (makeProfileDir) and changes only settings.json, adding the
// defaults of a newer build (workerSettings), and two keys of Claude Code's
// .claude.json (prepareClaudeState).
func profileDirPath() (string, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("the worker profile is in the home directory: %w", err)
	}
	return filepath.Join(home, ".config", "rewake", "claude", "worker"), home, nil
}

// profile is what the worker profile adds to one launch.
type profile struct {
	args []string
	// configDir is the run's CLAUDE_CONFIG_DIR, the service folder.
	configDir string
	notes     []string
	// enter says the run may enter a checkout: its settings let the mod's
	// worktree_enter call EnterWorktree.
	enter bool
}

// workerProfile builds the profile for a launch: none for main, and none
// under --command, whose wrapper decides the configuration it starts with —
// the entry into a checkout among it.
func workerProfile(request adapter.LaunchRequest, args []string) (profile, error) {
	if request.Role.ID == role.Main.ID {
		return profile{}, nil
	}
	if request.Command != "" {
		return profile{notes: []string{"the worker profile is not applied: the program named with --command decides the configuration Claude Code starts with"}, enter: request.WorktreeEntry}, nil
	}
	dir, home, err := profileDirPath()
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
	path, settings, wrote, err := workerSettings(dir, home)
	if err != nil {
		return profile{}, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return profile{}, fmt.Errorf("a worker's memory and trust are kept per project, and the working directory cannot be read: %w", err)
	}
	root := projectRoot(cwd)
	if err := prepareClaudeState(dir, root); err != nil {
		return profile{}, err
	}
	passed, enter, notes, err := runSettings(settings, dir, root, memoryKey(request), request.WorktreeEntry, request.RunDir)
	if err != nil {
		return profile{}, err
	}
	out := profile{args: []string{settingsFlag, passed}, configDir: dir, notes: notes, enter: enter}
	if wrote {
		out.notes = append(out.notes, fmt.Sprintf("the worker settings %s now hold rewake's defaults of version %d", path, defaultsVersion))
	}
	mcp := filepath.Join(dir, "mcp.json")
	if ok, err := present(mcp, false); err != nil {
		return profile{}, err
	} else if ok {
		out.args = append(out.args, mcpConfigFlag, mcp)
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
