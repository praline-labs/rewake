package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/praline-labs/rewake/internal/worktree"
)

// worktreeLanding is what land reports: which worktree, and what moved.
type worktreeLanding struct {
	Worktree string `json:"worktree"`
	worktree.Landing
}

func (l worktreeLanding) lines() []string {
	if l.Commits == 0 {
		return []string{fmt.Sprintf("nothing to land: %s already holds %s at %s", l.Target, l.Branch, shortCommit(l.New))}
	}
	how := "fast-forwarded in " + l.Checkout
	if l.Checkout == "" {
		how = "its ref moved, as it is checked out nowhere"
	}
	return []string{fmt.Sprintf("landed %s of %s into %s: %s..%s, %s",
		plural(l.Commits, "commit"), l.Branch, l.Target, shortCommit(l.Old), shortCommit(l.New), how)}
}

// worktreeFinish is what finish reports: the last landing and the removal.
type worktreeFinish struct {
	Landed worktreeLanding `json:"landed"`
	worktreeRemoval
}

func landWorktree(ctx *Context, call Call, record worktree.Record) error {
	landing, err := landRecord(call, record)
	if err != nil {
		return err
	}
	return printValue(ctx, landing, landing.lines)
}

// landRecord lands a checkout's branch and turns a refusal into the answer an
// agent acts on.
func landRecord(call Call, record worktree.Record) (worktreeLanding, error) {
	landing, err := worktree.Land(record, call.Flag("into", ""))
	if err != nil {
		return worktreeLanding{}, landRefusal(call, err)
	}
	return worktreeLanding{Worktree: record.Ref(), Landing: landing}, nil
}

// landRefusal is a refusal of land as the caller reads it: a wrong call exits
// 2, the state of the worktree or its repository 1.
func landRefusal(call Call, err error) error {
	var unusable *worktree.UnusableError
	var blocked *worktree.StateError
	var diverged *worktree.DivergedError
	var refused *worktree.RefusedError
	switch {
	case errors.As(err, &unusable):
		return &UsageError{Command: call.Command, Message: unusable.Error() + "."}
	case errors.As(err, &blocked):
		return &FailedError{Message: blocked.Error() + "."}
	case errors.As(err, &diverged):
		return &FailedError{Message: fmt.Sprintf("%s. rewake does not rewrite history: rebase the branch onto %s in the worktree — git -C %s rebase %s — then land again.",
			diverged.Error(), diverged.Target, diverged.Path, diverged.Target)}
	case errors.As(err, &refused):
		return &FailedError{Message: fmt.Sprintf("%s. Commit or stash those changes in %s, then land again.", refused.Error(), refused.Checkout)}
	}
	return &FailedError{Message: err.Error()}
}

// finishWorktree lands a checkout for the last time and removes it and its
// branch. Everything that would stop it is asked before anything moves, under
// the repository's lock the removal holds too, so a refused finish leaves the
// worktree, its branch and the target as they were.
func finishWorktree(ctx *Context, call Call, record worktree.Record) error {
	keep := func(current worktree.Record) error {
		reasons, err := finishReasons(current)
		if err != nil {
			return &FailedError{Message: fmt.Sprintf("cannot tell whether %s holds work: %v.", record.Ref(), err)}
		}
		if len(reasons) > 0 {
			return &FailedError{Message: fmt.Sprintf("%s is not finished: %s. Nothing was landed or removed.", record.Ref(), strings.Join(reasons, "; "))}
		}
		return nil
	}
	landed, err := worktree.Finish(record, call.Flag("into", ""), keep)
	var refused *FailedError
	var stuck *worktree.LandedError
	switch {
	case errors.As(err, &refused):
		return err
	case errors.As(err, &stuck):
		return &FailedError{Message: fmt.Sprintf("landed %s into %s, then could not remove it: %v", record.Ref(), stuck.Landing.Target, stuck.Err)}
	case err != nil:
		return landRefusal(call, err)
	}
	landing := worktreeLanding{Worktree: record.Ref(), Landing: landed}
	result := worktreeFinish{Landed: landing, worktreeRemoval: worktreeRemoval{Removed: record}}
	result.fateOfBranch(record)
	return printValue(ctx, result, func() []string {
		lines := append(landing.lines(), "removed "+record.Ref()+" at "+record.Path)
		return append(lines, result.branchLines()...)
	})
}

// finishReasons says what keeps a checkout from being finished: what rm would
// keep it for, and a checkout that left its branch, whose work land would not
// take.
func finishReasons(record worktree.Record) ([]string, error) {
	reasons, check, err := keepReasonsAndCheck(record)
	if err != nil {
		return nil, err
	}
	if !check.Missing && !check.Forgotten && check.Branch != record.Branch {
		now := "a detached HEAD"
		if check.Branch != "" {
			now = "the branch " + check.Branch
		}
		reasons = append(reasons, fmt.Sprintf("it is on %s, not on its branch %s, and land takes only that; git -C %s switch %s goes back", now, record.Branch, record.Path, record.Branch))
	}
	return reasons, nil
}
