package cli

import (
	"errors"
	"fmt"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/taskbrief"
	"github.com/praline-labs/rewake/internal/host/reviewcheckout"
	"github.com/praline-labs/rewake/internal/host/worktree"
)

// reviewCheckout gives the reviewer of brief id a detached checkout of the
// commit --commit names, from the repository main runs in, and binds the
// brief to review there (reviewcheckout.Give). The commit is main's word, not
// the writer's HEAD at send time: main often gives the writer its next brief
// before the review goes out, and a reviewer three times read a checkout that
// had moved on. A checkout the run's binding named before is removed when
// nothing in it would be lost; what stays, or could not be looked at, is said
// on stderr, the send being done all the same.
func reviewCheckout(ctx *Context, call Call, dir string, self registry.Session, recipient registry.Session, id int) error {
	reviewer := worktree.Owner{Name: recipient.Name, Room: recipient.Room, Epoch: recipient.Epoch(), Harness: recipient.Harness, Dir: dir}
	previous, err := reviewcheckout.Give(quietHomeOf(dir), id, self.CWD, call.Flags["commit"], reviewer)
	// Said even when the give failed: a take-back that kept the checkout it
	// made, for changes in it, leaves a path to remove.
	for _, path := range previous.Paths() {
		_, _ = fmt.Fprintf(ctx.Stderr, "Rewake: the reviewer's checkout %s stays: %s; rewake worktree rm removes it.\n", path, previous.Kept[path])
	}
	var step *reviewcheckout.StepError
	var unusable *worktree.UnusableError
	switch {
	case errors.As(err, &step) && step.Step == reviewcheckout.Root:
		return failf("no place for the reviewer's checkout: %v; nothing was sent.", err)
	case errors.As(err, &step) && step.Step == reviewcheckout.Commit:
		return &UsageError{Command: call.Command, Message: err.Error() + "; --commit takes a commit, branch or tag of the repository main runs in."}
	case errors.As(err, &unusable):
		return &UsageError{Command: call.Command, Message: err.Error() + "; nothing was sent."}
	case step != nil:
		return failf("could not make the reviewer's checkout: %v; nothing was sent.", err)
	case err != nil:
		return err
	}
	if previous.Err != nil {
		_, _ = fmt.Fprintf(ctx.Stderr, "Rewake: the reviewer's earlier checkout could not be released: %v; rewake worktree ls shows it.\n", previous.Err)
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
