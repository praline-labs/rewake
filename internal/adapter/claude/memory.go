package claude

import (
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A worker's memory (the owner's decision, October 19, 2026): Claude Code's
// own auto-memory, as main has it, but one per worker template and project,
// so the workers of one alias share theirs and no other's. Claude Code reads
// autoMemoryDirectory from --settings, an absolute path only — a relative
// one is ignored for its shared default (probed on 2.1.302).

// memoryKey names a worker's memory within its project: its type when the
// space raised it for one, what it learns being about the project and the
// kind of work rather than the model; else its template, as before types
// (main's decision, October 21, 2026). A template of a type's name shares
// that type's memory. Empty for a worker raised from no template.
func memoryKey(request adapter.LaunchRequest) string {
	if request.Template != "" && request.WorkType.Name != "" {
		return request.WorkType.Name
	}
	return request.Template
}

// templateName is what a template may be called to name a directory.
var templateName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// memoryDir is the memory of the template's workers in the project of the
// directory they start in: <folder>/memory/<project>/<template>.
func memoryDir(folder, cwd, template string) (string, error) {
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

// memorySettings is the worker's settings with its memory laid over them,
// its directory made private before Claude Code would make it; the settings
// as they are when the person's own autoMemoryEnabled false holds.
func memorySettings(settings map[string]any, folder, template string) (map[string]any, bool, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, false, fmt.Errorf("a worker's memory is kept per project, and the working directory cannot be read: %w", err)
	}
	dir, err := memoryDir(folder, cwd, template)
	if err != nil {
		return nil, false, err
	}
	overlaid, on := withMemory(settings, dir)
	if !on {
		return settings, false, nil
	}
	if err := makeMemoryDir(dir); err != nil {
		return nil, false, err
	}
	return overlaid, true, nil
}
