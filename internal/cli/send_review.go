package cli

import (
	"errors"
	"fmt"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/taskbrief"
	"github.com/praline-labs/rewake/internal/host/worktree"
)

// reviewCheckout gives the reviewer of brief id a detached checkout of the
// commit --commit names, from the repository main runs in, and binds the
// brief to review there. The commit is main's word, not the writer's HEAD at
// send time: main often gives the writer its next brief before the review
// goes out, and a reviewer three times read a checkout that had moved on.
// The brief is read first, so a brief that is gone or closed makes no
// checkout. A checkout the run's binding named before is removed when
// nothing in it would be lost.
func reviewCheckout(call Call, dir string, self registry.Session, recipient registry.Session, id int) error {
	home := quietHomeOf(dir)
	brief, err := taskbrief.Load(home, id)
	if err != nil {
		return err
	}
	if brief.IsClosed() {
		return &taskbrief.ClosedError{ID: id}
	}
	root, err := worktree.Root()
	if err != nil {
		return failf("no place for the reviewer's checkout: %v; nothing was sent.", err)
	}
	commit, err := worktree.Resolve(self.CWD, call.Flags["commit"])
	if err != nil {
		return &UsageError{Command: call.Command, Message: err.Error() + "; --commit takes a commit, branch or tag of the repository main runs in."}
	}
	review := worktree.Review{Brief: taskbrief.FormatID(id), Reviewer: worktree.Owner{
		Name: recipient.Name, Room: recipient.Room, Epoch: recipient.Epoch(), Harness: recipient.Harness, Dir: dir,
	}}
	record, err := worktree.CreateReview(root, self.CWD, commit, review)
	var unusable *worktree.UnusableError
	switch {
	case errors.As(err, &unusable):
		return &UsageError{Command: call.Command, Message: err.Error() + "; nothing was sent."}
	case err != nil:
		return failf("could not make the reviewer's checkout: %v; nothing was sent.", err)
	}
	previous, err := taskbrief.BindReview(home, id, taskbrief.Author{Name: recipient.Name, Epoch: recipient.Epoch()}, record.Path, commit)
	if err != nil {
		return err
	}
	if previous != "" {
		_, _ = worktree.ReleaseReviews(root, func(r worktree.Record) bool { return r.Path == previous })
	}
	return nil
}

// reviewCheckoutLine tells a reviewer where its checkout is, or nothing
// when the binding has none.
func reviewCheckoutLine(binding taskbrief.Binding) string {
	if binding.Checkout == "" {
		return ""
	}
	return fmt.Sprintf("Rewake: review in %s, a checkout of its own detached at %s; read and run the work there, not in the writer's checkout. It is removed when the brief closes or your run ends, unless it holds changes.", binding.Checkout, shortCommit(binding.Commit))
}

// releaseReviewCheckouts removes the reviewers' checkouts of brief id in
// the room at dir, its review bindings having ended with its close, and
// answers those that stay. A root that cannot be read leaves them to rm.
func releaseReviewCheckouts(dir string, id int) map[string]string {
	root, err := worktree.Root()
	if err != nil {
		return nil
	}
	kept, _ := worktree.ReleaseReviews(root, func(r worktree.Record) bool {
		return r.Review.Brief == taskbrief.FormatID(id) && r.Review.Reviewer.Dir == dir
	})
	if len(kept) == 0 {
		return nil
	}
	return kept
}
