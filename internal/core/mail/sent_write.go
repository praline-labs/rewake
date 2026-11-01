package mail

import (
	"context"

	"github.com/praline-labs/rewake/internal/core/registry"
)

// Send puts a new letter of this run into its recipient's mailbox. An
// addendum, a letter with AddendumTo set, goes in under the recipient's
// mailbox lock, after the task it adds to has been asked again
// (TaskToAddTo) whether a report on it can still come. An addendum in the
// mailbox before the report is read with its task and settled by that
// report, or read after it and owed on its own. It fails with a *Refusal,
// or with a *StepError at StepLock or StepWrite.
func (b Box) Send(parent context.Context, letter Message, target registry.Session) error {
	if letter.AddendumTo == "" {
		return failedAt(StepWrite, Put(b.dir, letter))
	}
	return b.underRecipientLock(parent, target.Name, func() error {
		if _, err := b.TaskToAddTo(letter, target); err != nil {
			return err
		}
		return failedAt(StepWrite, Put(b.dir, letter))
	})
}

// Edit is the replacement of a letter this run sent.
type Edit struct {
	// Named is the letter the call named. Current is what stood for it at
	// the caller's first look (CurrentSent).
	Named, Current Message
	// Target is the recipient's run.
	Target registry.Session
	// Replacement is the new letter. Its Replaces is set to the letter it
	// replaces.
	Replacement Message
}

// Edited is what an edit did: the letter it replaced and the replacement as
// written.
type Edited struct {
	Replaced, Replacement Message
}

// Replace withdraws the letter that stands for edit.Named now and writes the
// replacement in its place, under one hold of the recipient's mailbox lock
// (Withdraw). The replacement of an addendum adds to the same task, so the
// task is asked again first, as Send asks it. Then the letter is resolved
// again: an edit running beside this one may have replaced it since the
// first look, and this one replaces the letter that stands now rather than
// finding a tombstone. Replaced is the letter found under the lock, or
// edit.Current where the act stopped before it. A failure is a *Refusal, or a
// *StepError at StepLock, StepChain or StepWithdraw; the latter unwraps to
// Withdraw's own errors.
func (b Box) Replace(parent context.Context, edit Edit) (Edited, error) {
	done := Edited{Replaced: edit.Current, Replacement: edit.Replacement}
	err := b.underRecipientLock(parent, edit.Target.Name, func() error {
		if edit.Current.AddendumTo != "" {
			if _, err := b.TaskToAddTo(edit.Current, edit.Target); err != nil {
				return err
			}
		}
		current, err := b.CurrentSent(edit.Named)
		if err != nil {
			return err
		}
		if current.Withdrawn != nil {
			return &Refusal{Reason: EditOfWithdrawn, Letter: current}
		}
		done.Replaced, done.Replacement.Replaces = current, current.ID
		_, err = Withdraw(b.dir, current, &done.Replacement)
		return failedAt(StepWithdraw, err)
	})
	return done, err
}

// Withdrawn is what a withdrawal did.
type Withdrawn struct {
	// Letter is the letter withdrawn: the one named, or what stands for it
	// now. Where the act stopped before resolving it, it is the one named.
	Letter Message
	Result Withdrawal
	// Addenda are the task's addenda, each withdrawn in turn.
	Addenda []AddendumWithdrawn
}

// AddendumWithdrawn is the withdrawal of one addendum along with its task.
// Err is Withdraw's: ErrAlreadyRead for one already read, which stays owed.
type AddendumWithdrawn struct {
	Letter Message
	Result Withdrawal
	Err    error
}

// WithdrawSent takes back the letter that stands for named now, and every
// addendum of it, under the recipient's mailbox lock, so the letter found is
// the one withdrawn. An addendum is a task that only makes sense beside its
// task. Left standing, it would be read on its own and owe a report on work
// its sender took back. So the addenda are listed before anything is
// withdrawn, and a list that cannot be read stops it all (StepAddenda). A task
// withdrawn already still has its addenda taken, which lets a retry finish
// the rest. A failure is a *Refusal, or a *StepError at StepLock, StepChain,
// StepAddenda or StepWithdraw; the latter unwraps to Withdraw's own errors.
func (b Box) WithdrawSent(parent context.Context, named Message) (Withdrawn, error) {
	done := Withdrawn{Letter: named}
	err := b.underRecipientLock(parent, named.To, func() error {
		current, err := b.CurrentSent(named)
		done.Letter = current
		if err != nil {
			return err
		}
		addenda, err := AddendaOf(b.dir, current)
		if err != nil {
			return failedAt(StepAddenda, err)
		}
		if done.Result, err = Withdraw(b.dir, current, nil); err != nil {
			return failedAt(StepWithdraw, err)
		}
		for _, addendum := range addenda {
			result, err := Withdraw(b.dir, addendum, nil)
			done.Addenda = append(done.Addenda, AddendumWithdrawn{Letter: addendum, Result: result, Err: err})
		}
		return nil
	})
	return done, err
}
