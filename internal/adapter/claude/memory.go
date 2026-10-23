package claude

import (
	"fmt"
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

// memoryDir is the memory of the template's workers in the project whose
// root is given: <folder>/memory/<project>/<template>, the project named as
// rooms are named (state.RoomOf), a readable slug and a hash of the whole
// path.
func memoryDir(folder, root, template string) (string, error) {
	if !templateName.MatchString(template) {
		return "", fmt.Errorf("the alias %q cannot name a worker's memory directory: letters, digits, dot, dash or underscore", template)
	}
	project, _, err := state.RoomOf(root)
	if err != nil {
		return "", fmt.Errorf("a worker's memory is kept per project, and %s cannot name one: %w", root, err)
	}
	return filepath.Join(folder, "memory", project, template), nil
}

// projectRoot is the main repository's root of a directory: every worktree
// of one repository shares its memory, as Claude Code keys its own memory
// and its folder trust. Outside Git, the directory itself.
func projectRoot(cwd string) string {
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
	return root
}
