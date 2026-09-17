package codex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iiiokojiadbi/rewake/internal/harness"
)

// gitWriteFlags adds metadata roots without replacing the caller's roots or
// changing the selected permissions profile. Explicit sandbox policy still wins.
func gitWriteFlags(args []string) ([]string, string) {
	cwd, err := gitWorkingDirectory(args)
	if err != nil {
		return nil, gitWriteSkipped(err.Error())
	}
	if reason := gitWriteSkipReason(args); reason != "" {
		return nil, gitWriteSkipped(reason)
	}
	directories, err := gitMetadataDirectories(cwd)
	if err != nil {
		return nil, gitWriteSkipped(err.Error())
	}
	var flags []string
	for _, directory := range directories {
		flags = append(flags, "--add-dir", directory)
	}
	return flags, ""
}

func gitWriteSkipped(reason string) string {
	return "not granting Git metadata writes: " + reason + "; pass --add-dir for the actual Git metadata directories if commits are needed"
}

// The state directory in LaunchRequest is not the working directory. A caller
// may also change the latter with a harness flag, including its joined forms.
func gitWorkingDirectory(args []string) (string, error) {
	base, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot resolve the working directory: %w", err)
	}
	selected := base
	visible := harness.BeforeTerminator(args)
	for index := 0; index < len(visible); index++ {
		arg := visible[index]
		switch {
		case arg == "-C" || arg == "--cd":
			index++
			if index == len(visible) {
				return "", fmt.Errorf("%s has no working directory", arg)
			}
			selected = visible[index]
		case strings.HasPrefix(arg, "--cd="):
			selected = strings.TrimPrefix(arg, "--cd=")
		case strings.HasPrefix(arg, "-C") && len(arg) > 2:
			selected = strings.TrimPrefix(arg[2:], "=")
		}
	}
	if selected == "" {
		return "", fmt.Errorf("the working directory is empty")
	}
	if !filepath.IsAbs(selected) {
		selected = filepath.Join(base, selected)
	}
	resolved, err := filepath.EvalSymlinks(selected)
	if err != nil {
		return "", fmt.Errorf("cannot resolve working directory %s: %w", selected, err)
	}
	return resolved, nil
}

// A new managed worktree needs its not-yet-allocated private gitdir; remote
// paths are not local. Continuations are handled before requesting these flags.
func gitWriteSkipReason(args []string) string {
	visible := harness.BeforeTerminator(args)
	for index := 0; index < len(visible); index++ {
		arg := visible[index]
		switch arg {
		case "-c", "--config", "-C", "--cd", "-p", "--profile", "--add-dir", "-m", "--model":
			index++
			continue
		case "--worktree":
			return "the new worktree's private Git metadata path is allocated later; create the worktree first and launch from its directory"
		case "--remote":
			return "the working repository is selected by " + arg
		}
		if strings.HasPrefix(arg, "--remote=") {
			return "the working repository is selected by --remote"
		}
	}
	return ""
}
