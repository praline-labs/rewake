package codex

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iiiokojiadbi/rewake/internal/harness"
)

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

// worktreeFlag is Codex's switch for a worktree of its own, which rewake takes
// (harness.WorktreeHarness).
const worktreeFlag = "--worktree"

func (codexHarness) WorktreeFlag() string { return worktreeFlag }

// LaunchDirectory is the directory -C or --cd chose, or the current one, and
// the arguments without them: the launch starts in the checkout instead.
func (codexHarness) LaunchDirectory(args []string) (string, []string, error) {
	dir, err := gitWorkingDirectory(args)
	if err != nil {
		return "", nil, err
	}
	return dir, harness.WithoutFlag(args, "--cd", "-C"), nil
}

// localArgumentsRequired refuses a launch whose arguments name a server or a
// configuration of their own.
const localArgumentsRequired = "session-owned app-server requires local arguments without --remote, --profile or --oss/--local-provider; select a configuration explicitly before launching"

// WorktreeRefusal refuses what the launch itself would refuse, and a
// continued conversation. resume and fork carry on in the directory the
// conversation was started in: the terminal sends thread/resume with no cwd,
// and the server keeps the old one, while the session's workspace roots are
// already the checkout's (seen on 0.155.1). The model would work in one place
// with its rights in another.
func (codexHarness) WorktreeRefusal(args []string) error {
	if harness.HasFlag(args, "--remote") || hasProfile(args) || harness.HasFlag(args, "--oss") || harness.HasFlag(args, "--local-provider") {
		return errors.New(localArgumentsRequired)
	}
	if mode := continuationWord(args); mode != "" {
		return fmt.Errorf("%s makes a new checkout for a new conversation, and %s continues one in the directory it was started in, which Codex keeps: the session would work outside the checkout its permissions name. Start a new conversation with %s, or %s without %s; a prompt that is the word %s itself goes after --", worktreeFlag, mode, worktreeFlag, mode, worktreeFlag, mode)
	}
	return nil
}

// continuationWord is resume or fork when either appears as a whole argument
// before the terminator, or "". It does not follow Codex's grammar, which lets
// a prompt precede the subcommand and an image's joined value end where a
// separated one runs on: a parser that trails it misses a form the terminal
// accepts, and each miss is a checkout the conversation never works in. A word
// that was an option's value or the whole prompt is refused as well; the
// refusal names the way round, and a prompt that must be the bare word goes
// after --.
func continuationWord(args []string) string {
	for _, arg := range harness.BeforeTerminator(args) {
		if arg == "resume" || arg == "fork" {
			return arg
		}
	}
	return ""
}
