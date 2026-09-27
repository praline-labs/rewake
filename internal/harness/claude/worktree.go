package claude

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iiiokojiadbi/rewake/internal/harness"
)

// worktreeFlag is the long spelling of Claude Code's own worktree flag, which
// rewake takes (harness.WorktreeHarness) so a Claude Code session gets the same
// checkout, branch and record as a Codex one. The short -w stays Claude Code's:
// its own worktree inside the repository, on a branch worktree-<name>, which it
// removes itself on exit (docs/research-launch.md).
const worktreeFlag = "--worktree"

func (claudeHarness) WorktreeFlag() string { return worktreeFlag }

// LaunchDirectory is the current directory: Claude Code has no flag that
// chooses where it works, so the arguments come back as they are.
func (claudeHarness) LaunchDirectory(args []string) (string, []string, error) {
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

// continuationFlags continue a conversation rather than start one, as
// `claude --help` lists them on 2.1.280: the most recent one of the directory,
// one by id or picker, one linked to a pull request, a teleported one, and a
// fork of any of them.
var continuationFlags = []string{"--continue", "-c", "--resume", "-r", "--from-pr", "--teleport", "--fork-session"}

// WorktreeRefusal refuses Claude Code's own worktree beside rewake's, and a
// continued conversation. Claude Code keeps a conversation with the directory
// it ran in: --continue in a new checkout finds none there, and --resume of
// one begun elsewhere sends the person back to its directory — or, for one
// begun in Claude Code's own worktree, changes into that — rather than work
// in the checkout (read in the reference source, not run).
func (claudeHarness) WorktreeRefusal(args []string) error {
	visible := harness.BeforeTerminator(args)
	for _, arg := range visible {
		if _, _, ok := harness.MatchFlag(arg, "-w"); ok {
			return fmt.Errorf("%s is rewake's worktree and -w is Claude Code's own; a launch has one worktree, so give one of them", worktreeFlag)
		}
		if _, _, ok := harness.MatchFlag(arg, "--tmux"); ok {
			return errors.New("--tmux opens Claude Code's own worktree in tmux and needs its -w; rewake's " + worktreeFlag + " has no tmux")
		}
	}
	if word := claudeContinuation(visible); word != "" {
		return fmt.Errorf("%s makes a new checkout for a new conversation, and %s continues one kept with the directory it was started in: in the checkout Claude Code finds none, or leads back out. Start a new conversation with %s, or %s; a prompt that is the word itself goes after --",
			worktreeFlag, word, worktreeFlag, harness.ContinueInWorktree("rewake claude", word))
	}
	return nil
}

// claudeContinuation is the first continuation flag among the arguments, or
// "". A cluster of short flags counts when it holds c or r, as -pc or -rID:
// Claude Code's parser reads -pc as -p -c. Like Codex's refusal it asks of the
// words rather than of the grammar: a value that happens to be one is refused
// too, and the refusal says where such text goes.
func claudeContinuation(args []string) string {
	for _, arg := range args {
		for _, flag := range continuationFlags {
			if _, _, ok := harness.MatchFlag(arg, flag); ok {
				return flag
			}
		}
		if cluster, ok := strings.CutPrefix(arg, "-"); ok && len(cluster) > 1 && !strings.HasPrefix(cluster, "-") && strings.ContainsAny(cluster, "cr") {
			return arg
		}
	}
	return ""
}
