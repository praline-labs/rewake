package claude

import (
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// A worker's memory (the owner's decision, October 19, 2026): Claude Code's
// own auto-memory, as main has it, but one per worker template and project,
// so the workers of one alias share theirs and no other's. Claude Code reads
// autoMemoryDirectory from --settings, an absolute path only — a relative
// one is ignored for its shared default (probed on 2.1.302).

// defaultTemplate names the memory of workers opened by --harness, with no
// alias.
const defaultTemplate = "default"

// templateName is what a template may be called to name a directory.
var templateName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// memoryDir is the memory of the template's workers in the project of the
// directory they start in: <folder>/memory/<project>/<template>.
func memoryDir(folder, cwd, template string) (string, error) {
	if template == "" {
		template = defaultTemplate
	}
	if !templateName.MatchString(template) {
		return "", fmt.Errorf("the alias %q cannot name a worker's memory directory: letters, digits, dot, dash or underscore", template)
	}
	project, err := memoryProject(cwd)
	if err != nil {
		return "", err
	}
	return filepath.Join(folder, "memory", project, template), nil
}

// memoryProject names the project of a directory as rooms are named
// (state.RoomOf), a readable slug and a hash of the whole path, of the main
// repository's root: every worktree of one repository shares its memory, as
// Claude Code keys its own. Outside Git, the directory itself.
func memoryProject(cwd string) (string, error) {
	root := cwd
	out, err := exec.Command("git", "-C", cwd, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if common := strings.TrimSpace(string(out)); err == nil && common != "" {
		// <root>/.git for a repository with a working tree; a bare or
		// separate Git directory is its own key.
		root = common
		if filepath.Base(common) == ".git" {
			root = filepath.Dir(common)
		}
	}
	project, _, err := state.RoomOf(root)
	if err != nil {
		return "", fmt.Errorf("a worker's memory is kept per project, and %s cannot name one: %w", root, err)
	}
	return project, nil
}

// withMemory is the worker's settings with its memory laid over them, on and
// in dir; nothing else changes. A person's own autoMemoryEnabled false
// holds: no memory.
func withMemory(settings map[string]any, dir string) (map[string]any, bool) {
	if settings["autoMemoryEnabled"] == false {
		return nil, false
	}
	out := maps.Clone(settings)
	out["autoMemoryEnabled"] = true
	out["autoMemoryDirectory"] = dir
	return out, true
}

// launchSettings is the --settings a worker's launch passes: the folder's
// settings.json, or, with memory on, a copy in the run's directory with the
// two memory keys laid over it, since --settings is single-use. The memory
// directory is made private before Claude Code would make it. With no run
// directory to write in, the worker starts without memory, noted.
func launchSettings(path string, settings map[string]any, folder, runDir, template string) (string, []string, error) {
	if runDir == "" {
		return path, []string{"no worker memory: the launch has no run directory to write its settings in"}, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", nil, fmt.Errorf("a worker's memory is kept per project, and the working directory cannot be read: %w", err)
	}
	dir, err := memoryDir(folder, cwd, template)
	if err != nil {
		return "", nil, err
	}
	overlaid, on := withMemory(settings, dir)
	if !on {
		return path, nil, nil
	}
	if err := makeMemoryDir(dir); err != nil {
		return "", nil, err
	}
	data, err := encodeSettings(overlaid)
	if err != nil {
		return "", nil, err
	}
	// A run whose directory cannot be written fails its mod at the start;
	// the memory alone stops nothing.
	run := filepath.Join(runDir, "worker-settings.json")
	err = state.EnsureSubdir(runDir)
	if err == nil {
		err = state.WriteFile(run, data, 0o600)
	}
	if err != nil {
		return path, []string{"no worker memory: its settings could not be written to the run directory: " + err.Error()}, nil
	}
	return run, nil, nil
}
