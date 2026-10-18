package pi

import (
	"fmt"
	"os"
	"path/filepath"
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
	if flag := continued(args); flag != "" {
		return fmt.Errorf("%s names an existing conversation; continue it from its checkout without --worktree", flag)
	}
	return nil
}
