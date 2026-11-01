package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// addendumRoot resolves send --to: the task this message adds to. An addendum
// is a task of its own, so it is announced at once and read into the same
// wait as the task, and one report settles both; it is refused where no such
// report can come, since a correction nobody will act on is worse than a new
// task. This look gives the quick refusal; mail.Box.Send asks again under the
// recipient's mailbox lock.
func addendumRoot(call Call, dir string, kind messageKind, target registry.Session) (string, error) {
	reference := call.Flag("to", "")
	if reference == "" {
		return "", nil
	}
	if kind.kind != mail.Task {
		return "", &UsageError{Command: call.Command, Message: "--to adds to a task already sent and is a task itself; it excludes " + kind.flag.Flag + "."}
	}
	self, epoch, named, err := sentBySelf(call, dir, reference, fmt.Sprintf("rewake send %s \"...\" --to <id>", target.Name))
	if err != nil {
		return "", err
	}
	root, err := mail.Open(dir, self.Name, epoch).TaskToAddTo(named, target)
	if err != nil {
		return "", refusalOf(err, target)
	}
	return root.ID, nil
}

// writeSent puts a new message into its recipient's mailbox; an addendum goes
// in under the lock, with its task asked again (mail.Box.Send).
func writeSent(dir string, self registry.Session, epoch string, message mail.Message, target registry.Session) error {
	err := mail.Open(dir, self.Name, epoch).Send(context.Background(), message, target)
	var refusal *mail.Refusal
	switch {
	case err == nil:
		return nil
	case errors.As(err, &refusal):
		return refusalOf(err, target)
	}
	return failf("could not write the message into the mailbox of %s: %v", target.Name, lockText(err, target.Name))
}

// refusalOf words a mail.Refusal of an act on a sent letter, with the command
// to run instead; target is the recipient the call named. Any other error is
// answered as it is.
func refusalOf(err error, target registry.Session) error {
	var refusal *mail.Refusal
	if !errors.As(err, &refusal) {
		return err
	}
	letter := refusal.Letter
	next := fmt.Sprintf("rewake send %s \"...\"", target.Name)
	switch refusal.Reason {
	case mail.ReplacementLost:
		return failf("your %s %s was replaced by %s, which is no longer kept; see what you sent with: rewake inbox --awaited", sentKind(letter), letter.ID, refusal.Leads)
	case mail.TaskLost:
		return failf("the task %s, which %s leads to, is no longer kept; send a new task with: %s", refusal.Leads, letter.ID, next)
	case mail.TaskUnknown:
		// The task may have been answered or replaced, and an addition on a
		// guess could owe a second report.
		return failf("could not read where your %s %s stands (%v), so nothing was added to it; run the same command again", mail.KindOf(letter), letter.ID, refusal.Err)
	case mail.OtherRecipient:
		return failf("%s went to %s, not %s; add to it with: rewake send %s \"...\" --to %s", letter.ID, letter.To, target.Name, letter.To, shortRef(letter.ID))
	case mail.TaskWithdrawn:
		return failf("your %s %s was withdrawn; send a new task with: %s", letter.Withdrawn.Kind, letter.ID, next)
	case mail.OwesNothing:
		return failf("%s is a %s, which owes nothing to add to; send another with: %s --notify", letter.ID, mail.KindOf(letter), next)
	case mail.Reported:
		return failf("%s has already reported on your %s %s; send the addition as a new task: %s", target.Name, mail.KindOf(letter), letter.ID, next)
	case mail.RecipientGone:
		return failf("the session run your %s %s was written for has ended, so no report on it is coming; send a new task with: %s", mail.KindOf(letter), letter.ID, next)
	case mail.NotDelivered:
		return failf("your %s %s was not delivered to %s; send the task again, with the addition, as: %s", mail.KindOf(letter), letter.ID, target.Name, next)
	case mail.EditOfWithdrawn:
		return editedWithdrawn(letter)
	}
	return failf("%v", err)
}

// sentKind is what a letter was sent as, a tombstone's included.
func sentKind(letter mail.Message) mail.Kind {
	if letter.Withdrawn != nil {
		return letter.Withdrawn.Kind
	}
	return mail.KindOf(letter)
}

// lockText says a recipient's mailbox stayed busy for as long as a reader
// waits for it; any other error is answered as it is.
func lockText(err error, name string) error {
	if errors.Is(err, state.ErrMailboxBusy) {
		return fmt.Errorf("the mailbox of %s stayed busy for %s", name, mail.ReadLockWait)
	}
	return err
}
