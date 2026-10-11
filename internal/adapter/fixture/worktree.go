//go:build rewakefixture

package fixture

import (
	"fmt"
	"os"
	"path/filepath"
)

// The fixture takes rewake's worktree flag (harness.WorktreeHarness), so the
// worktree path the core owns — the checkout, its branch and record, and rm,
// land and finish around a running session — has its case on the gate
// column. It has no worktree of its own to refuse beside rewake's, and no
// conversation to continue.

const worktreeFlag = "--worktree"

func (fixtureHarness) WorktreeFlag() string { return worktreeFlag }

// WorktreeNameSpaced: the fixture's flag takes its name only after =.
func (fixtureHarness) WorktreeNameSpaced() bool { return false }

// LaunchDirectory is the current directory: the fixture has no flag that
// chooses where it works.
func (fixtureHarness) LaunchDirectory(args []string) (string, []string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", nil, fmt.Errorf("cannot resolve the working directory: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", nil, fmt.Errorf("cannot resolve working directory %s: %w", dir, err)
	}
	return resolved, args, nil
}

// WorktreeRefusal refuses nothing: no argument of the fixture's leads out of
// the checkout.
func (fixtureHarness) WorktreeRefusal([]string) error { return nil }
