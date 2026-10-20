package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/host/worktree"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The outcomes of rewake worktree enter.
const (
	// enterCreated: a checkout was made and claimed for this run.
	enterCreated = "created"
	// enterReused: this run already owns one, made by an earlier call.
	enterReused = "reused"
	// enterAlready: the run was started in the checkout it owns.
	enterAlready = "already"
)

// worktreeEntry is what rewake worktree enter answers: the one checkout of
// this run, for its harness's tool to move the session into.
type worktreeEntry struct {
	Outcome  string         `json:"outcome"`
	Checkout statusCheckout `json:"checkout"`
	// Skipped names what .worktreeinclude asked for and was not copied.
	Skipped []string `json:"skipped,omitempty"`
}

// enterWorktree makes the one checkout of a worker whose template lets it
// enter one, from the commit its session is at, and claims it for its run, so
// its space's close releases it as a launch's own; a second call answers the
// same checkout. A change: it writes the checkout's record under the room's
// lease. Moving the session is the harness's tool's, not this command's.
func enterWorktree(ctx *Context, call Call, root string) error {
	if os.Getenv(adapter.WorktreeEnterEnv) != "1" {
		return &UsageError{Command: call.Command, Message: "enter is for a worker whose template says enter_worktree = true, through its worktree_enter tool; a launch with --worktree, or space open --worktree, starts one in a checkout of its own."}
	}
	name := ""
	if len(call.Positionals) > 1 {
		name = call.Positionals[1]
	}
	dir, err := state.Dir()
	if err != nil {
		return &UsageError{Command: call.Command, Message: err.Error()}
	}
	self, epoch, err := ownRunIn(ctx.access, dir)
	if errors.Is(err, errNotASession) {
		return &UsageError{Command: call.Command, Message: "enter makes a checkout for the session it runs in; this shell runs in none."}
	}
	if err != nil {
		return failf("%v", err)
	}
	owned, claimed, err := claimedBy(root, dir, self, epoch)
	if err != nil {
		return failf("could not read the worktrees under %s: %v", root, err)
	}
	if claimed {
		if name != "" && name != owned.Name && name != owned.Ref() {
			return &UsageError{Command: call.Command, Message: fmt.Sprintf(
				"one checkout per worker: yours is %s at %s. Work there, or ask main for a new space for work that needs another.", owned.Ref(), owned.Path)}
		}
		outcome := enterReused
		if worktree.Within(self.CWD, owned.Path) {
			outcome = enterAlready
		}
		return printEntry(ctx, worktreeEntry{Outcome: outcome, Checkout: checkoutOf(owned)})
	}
	from, err := os.Getwd()
	if err != nil {
		return failf("the working directory cannot be read: %v", err)
	}
	record, err := worktree.Create(root, from, name)
	var exists *worktree.ExistsError
	var taken *worktree.BranchTakenError
	var unusable *worktree.UnusableError
	switch {
	case errors.As(err, &exists):
		return &UsageError{Command: call.Command, Message: exists.Error() + "; give another name, or none for a generated one."}
	case errors.As(err, &taken):
		return &UsageError{Command: call.Command, Message: taken.Error() + ", and a worktree gets a new branch of its name; give another name, or none for a generated one."}
	case errors.As(err, &unusable):
		return &UsageError{Command: call.Command, Message: unusable.Error() + "."}
	case err != nil:
		return &FailedError{Message: err.Error()}
	}
	record, err = worktree.Claim(record, worktree.Owner{Name: self.Name, Room: self.Room, Epoch: epoch, Harness: self.Harness, Dir: dir})
	if err != nil {
		// Unclaimed, no close would find it: taken back as a failed
		// launch's is, kept only when something in it would be lost.
		if why := (&launchCheckout{record: record}).takeBack(); why != "" {
			return failf("the worktree %s at %s could not be claimed for this session (%v), and stays: %s", record.Ref(), record.Path, err, why)
		}
		return failf("the worktree could not be claimed for this session, and was removed: %v", err)
	}
	return printEntry(ctx, worktreeEntry{Outcome: enterCreated, Checkout: checkoutOf(record), Skipped: record.Skipped})
}

func checkoutOf(record worktree.Record) statusCheckout {
	return statusCheckout{Ref: record.Ref(), Path: record.Path, Branch: record.Branch, Base: record.Commit, Source: record.Source}
}

func printEntry(ctx *Context, entry worktreeEntry) error {
	return printValue(ctx, entry, entry.lines)
}

func (e worktreeEntry) lines() []string {
	c := e.Checkout
	lines := []string{fmt.Sprintf("%s: the worktree %s at %s, branch %s, made from %s at %s", e.Outcome, c.Ref, c.Path, orDash(c.Branch), c.Source, shortCommit(c.Base))}
	for _, skipped := range e.Skipped {
		lines = append(lines, worktree.IncludeFile+" did not copy "+filepath.Clean(skipped))
	}
	return lines
}
