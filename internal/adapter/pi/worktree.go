package pi

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/praline-labs/rewake/internal/adapter"
)

func (harness) WorktreeFlag() string     { return worktreeFlag }
func (harness) WorktreeNameSpaced() bool { return false }

func (harness) LaunchDirectory(args []string) (string, []string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", nil, err
	}
	dir, err = filepath.EvalSymlinks(dir)
	return dir, args, err
}

func (harness) WorktreeRefusal(args []string) error {
	for _, flag := range []string{"--session", "--continue", "-c", "--resume", "-r", "--fork"} {
		if adapter.HasFlag(args, flag) {
			return fmt.Errorf("%s names an existing conversation; continue it from its checkout without --worktree", flag)
		}
	}
	// --session-id can create a conversation, unlike the flags above. A
	// space supplies a fresh random id, not the id of an earlier session.
	return nil
}
