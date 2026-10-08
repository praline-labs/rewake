package claude

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/praline-labs/rewake/internal/harness"
)

// worktreeFlag is the long spelling of Claude Code's own worktree flag, which
// rewake takes (harness.WorktreeHarness) so a Claude Code session gets the
// checkout, branch and record every harness's does. The short -w stays Claude Code's:
// its own worktree inside the repository, on a branch worktree-<name>, which it
// removes itself on exit (docs/research-launch.md).
const worktreeFlag = "--worktree"

func (claudeHarness) WorktreeFlag() string { return worktreeFlag }

// WorktreeNameSpaced: Claude Code's own flag is --worktree [name], so a person
// used to it writes the name after a space.
func (claudeHarness) WorktreeNameSpaced() bool { return true }

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
// fork of any of them. --session-id is not among them: it names the id of a new
// conversation, and Claude Code refuses an id already in use
// (docs/research-worktree.md).
var continuationFlags = []string{"--continue", "-c", "--resume", "-r", "--from-pr", "--teleport", "--fork-session"}

// continuationCommands open a background session again, and it works where it
// was started (`claude --help`, 2.1.280).
var continuationCommands = []string{"attach", "respawn"}

// valueLetters are the short flags that take a value, as `claude --help` lists
// them on 2.1.280: -d [filter], -n <name>, -r [value], -w [name]. In a cluster
// such a letter ends the flags, and what follows is its value.
const valueLetters = "dnrw"

// WorktreeRefusal refuses Claude Code's own worktree beside rewake's, a cloud
// session — --cloud, or --environment, which makes one on that environment —
// and a continued conversation. Claude Code keeps a conversation with
// the directory it ran in: --continue in a new checkout finds none there, and
// --resume of one begun elsewhere sends the person back to its directory — or,
// for one begun in Claude Code's own worktree, changes into that — rather than
// work in the checkout (read in the reference source, not run).
func (claudeHarness) WorktreeRefusal(args []string) error {
	visible := harness.BeforeTerminator(args)
	for _, arg := range visible {
		if _, _, ok := harness.MatchFlag(arg, "-w"); ok || strings.ContainsRune(shortLetters(arg), 'w') {
			return fmt.Errorf("%s is rewake's worktree and -w is Claude Code's own; a launch has one worktree, so give one of them", worktreeFlag)
		}
		if _, _, ok := harness.MatchFlag(arg, "--tmux"); ok {
			return errors.New("--tmux opens Claude Code's own worktree in tmux and needs its -w; rewake's " + worktreeFlag + " has no tmux")
		}
		for _, cloud := range []string{"--cloud", "--environment"} {
			if _, _, ok := harness.MatchFlag(arg, cloud); ok {
				return errors.New(cloud + " starts a cloud session or attaches to one, and it works in none of this machine's checkouts; rewake's " + worktreeFlag + " is a checkout here, so launch one or the other")
			}
		}
	}
	if word := claudeContinuation(visible); word != "" {
		return fmt.Errorf("%s makes a new checkout for a new conversation, and %s continues one kept with the directory it was started in: in the checkout Claude Code finds none, or leads back out. Start a new conversation with %s, or %s; a prompt that is the word itself goes after --",
			worktreeFlag, word, worktreeFlag, harness.ContinueInWorktree("rewake claude", word))
	}
	return nil
}

// claudeContinuation is the first continuation among the arguments, or "": a
// flag, a cluster of short flags in which c or r acts as a flag, or a
// subcommand word. It asks of the words rather than of the grammar: a value
// that happens to be one is refused too, and the refusal says where such text
// goes. After -- a word is the prompt's; Claude Code does not take it for a
// subcommand there (seen on 2.1.280).
func claudeContinuation(args []string) string {
	for _, arg := range args {
		for _, flag := range continuationFlags {
			if _, _, ok := harness.MatchFlag(arg, flag); ok {
				return flag
			}
		}
		if strings.ContainsAny(shortLetters(arg), "cr") || slices.Contains(continuationCommands, arg) {
			return arg
		}
	}
	return ""
}

// shortLetters is the letters of a cluster of short flags that act as flags:
// Claude Code's parser reads -pc as -p -c. A letter taking a value ends them,
// so -dcache is -d with the filter cache and -ncircle a name, not -c; -rID is
// -r with its value, and counts.
func shortLetters(arg string) string {
	cluster, ok := strings.CutPrefix(arg, "-")
	if !ok || strings.HasPrefix(cluster, "-") {
		return ""
	}
	for i := 0; i < len(cluster); i++ {
		if strings.IndexByte(valueLetters, cluster[i]) >= 0 {
			return cluster[:i+1]
		}
	}
	return cluster
}
