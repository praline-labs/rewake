package launch

import (
	"fmt"
	"slices"

	"github.com/praline-labs/rewake/internal/adapter"
)

// Worker is an ordinary launch owned by a space. Its name, role and room are
// not words of it: the launch reads them from the space's record, named by
// the space host's REWAKE_SPACE. Worktree is empty when no checkout was
// requested.
type Worker struct {
	Command      string
	NoIntro      bool
	Args         []string
	Worktree     string
	Conversation string
}

// WorkerArgs builds the complete command the detached space host runs. It
// adds harness flags before a prompt terminator, preserving literal input.
func WorkerArgs(h adapter.Harness, worker Worker) ([]string, error) {
	var words []string
	if worker.Command != "" {
		words = append(words, "--command", worker.Command)
	}
	if worker.NoIntro {
		words = append(words, "--no-intro")
	}
	words = append(words, h.ID())
	args := slices.Clone(worker.Args)
	if worker.Worktree != "" {
		taker, ok := h.(adapter.WorktreeHarness)
		if !ok {
			return nil, fmt.Errorf("%s makes no worktree through rewake; open the space without --worktree", h.Title())
		}
		flag := taker.WorktreeFlag()
		if worker.Worktree != "true" {
			flag += "=" + worker.Worktree
		}
		visible := adapter.BeforeTerminator(args)
		args = append(MergeArguments(visible, []string{flag}, h.SingleUseFlags()), args[len(visible):]...)
	}
	if offer := h.Offers().Conversation; offer.Names() {
		if adapter.HasFlag(args, offer.Flag) {
			return nil, fmt.Errorf("rewake chooses the worker's %s; remove it from the alias or harness arguments", offer.Flag)
		}
		args = adapter.AddFlags(args, offer.Flag, worker.Conversation)
	}
	return append(words, args...), nil
}
