// Package reviewcheckout owns a reviewer's checkout from the brief's
// assignment to its release: the order of the create, the bind and the
// release of the one before, and the rule of whose checkout it is. The CLI
// keeps the flags, main's authorization and the printing, taskbrief its
// rights and lock, worktree the git safety of a release.
package reviewcheckout

import (
	"errors"
	"fmt"
	"slices"

	"github.com/praline-labs/rewake/internal/core/taskbrief"
	"github.com/praline-labs/rewake/internal/host/worktree"
)

// Step names where Give failed, for a caller that words each differently.
type Step int

const (
	// Root says the place for checkouts could not be found.
	Root Step = iota + 1
	// Commit says the commit named is none in the repository.
	Commit
	// Checkout says git could not make the checkout.
	Checkout
)

// StepError is a failure of one step of Give. Bind's and Load's errors come
// back as they are, so the caller can tell a closed brief from a missing one.
type StepError struct {
	Step Step
	Err  error
}

func (e *StepError) Error() string { return e.Err.Error() }
func (e *StepError) Unwrap() error { return e.Err }

// Outcome is what a release left: the checkouts that stay, each with why, and
// the error that kept it from looking at some of them. A release that fails
// never fails what ended; its caller prints both.
type Outcome struct {
	Kept map[string]string
	Err  error
}

// Paths are the kept checkouts in order.
func (o Outcome) Paths() []string {
	paths := make([]string, 0, len(o.Kept))
	for path := range o.Kept {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	return paths
}

// Reviewer is the run a brief is given to review.
type Reviewer = worktree.Owner

// Give gives brief id to the reviewer to review in a detached checkout of rev,
// from the repository holding from: the brief is read first, so one that is
// gone or closed makes no checkout; then the checkout is made, the brief bound
// to it, and the checkout the reviewer's binding named before released. A
// bind that fails takes the checkout just made back, and the brief is not
// left with one bound to nothing. The outcome is the previous one's release.
func Give(home taskbrief.Home, id int, from, rev string, reviewer Reviewer) (Outcome, error) {
	brief, err := taskbrief.Load(home, id)
	if err != nil {
		return Outcome{}, err
	}
	if brief.IsClosed() {
		return Outcome{}, &taskbrief.ClosedError{ID: id}
	}
	root, err := worktree.Root()
	if err != nil {
		return Outcome{}, &StepError{Root, err}
	}
	commit, err := worktree.Resolve(from, rev)
	if err != nil {
		return Outcome{}, &StepError{Commit, err}
	}
	review := worktree.Review{Brief: taskbrief.FormatID(id), Reviewer: reviewer}
	record, err := worktree.CreateReview(root, from, commit, review)
	if err != nil {
		return Outcome{}, &StepError{Checkout, err}
	}
	previous, err := taskbrief.BindReview(home, id, taskbrief.Author{Name: reviewer.Name, Epoch: reviewer.Epoch}, record.Path, commit)
	if err != nil {
		undone := release(root, func(r worktree.Record) bool { return r.Path == record.Path })
		if undone.Err != nil {
			err = errors.Join(err, fmt.Errorf("the checkout %s just made could not be released: %w", record.Path, undone.Err))
		}
		return Outcome{}, err
	}
	if previous == "" {
		return Outcome{}, nil
	}
	return release(root, func(r worktree.Record) bool { return r.Path == previous }), nil
}

// ReleaseBrief releases the checkouts of brief id given by the room at dir,
// its review bindings having ended with its close.
func ReleaseBrief(dir string, id int) Outcome {
	return releaseWhere(func(r worktree.Record) bool {
		return r.Review.Brief == taskbrief.FormatID(id) && r.Review.Reviewer.Dir == dir
	})
}

// ReleaseRun releases the checkouts given to the run of a reviewer in the
// room at dir, its run having ended.
func ReleaseRun(dir, run string) Outcome {
	if run == "" {
		return Outcome{}
	}
	return releaseWhere(func(r worktree.Record) bool {
		return r.Review.Reviewer.Epoch == run && r.Review.Reviewer.Dir == dir
	})
}

func releaseWhere(pick func(worktree.Record) bool) Outcome {
	root, err := worktree.Root()
	if err != nil {
		return Outcome{Err: fmt.Errorf("no place for checkouts is known: %w", err)}
	}
	return release(root, pick)
}

// release removes each review checkout pick names where worktree.Release
// would: nothing in it lost.
func release(root string, pick func(worktree.Record) bool) Outcome {
	kept, err := worktree.ReleaseReviews(root, pick)
	if len(kept) == 0 {
		kept = nil
	}
	return Outcome{Kept: kept, Err: err}
}
