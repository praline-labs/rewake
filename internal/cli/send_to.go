package cli

import (
	"errors"
	"fmt"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
)

// addendumRoot resolves send --to: the task this message adds to. An addendum
// is a task of its own, so it is announced at once and read into the same
// wait as the task, and one report settles both; it is refused where no such
// report can come, since a correction nobody will act on is worse than a new
// task.
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
	root, err := rootOf(dir, self, epoch, named, target)
	if err != nil {
		return "", err
	}
	if err := addendumRefusal(dir, self, epoch, root, target); err != nil {
		return "", err
	}
	return root.ID, nil
}

// rootOf is the task something is added to: an addendum's task — an addendum
// to an addendum adds to the task they both belong to — and, for a letter
// rewake edit replaced, what replaced it, through every edit since. Two steps
// at most: from an edited addendum to its replacement, then to its task.
func rootOf(dir string, self registry.Session, epoch string, message mail.Message, target registry.Session) (mail.Message, error) {
	current := message
	for range 2 {
		id := current.ID
		if current.AddendumTo != "" {
			id = current.AddendumTo
		}
		id, err := mail.CurrentTask(dir, current.To, id)
		if err != nil {
			return mail.Message{}, unreadableTask(message, err)
		}
		if id == current.ID {
			return current, nil
		}
		matches, err := mail.SentMatching(dir, self.Name, epoch, id)
		if err != nil {
			return mail.Message{}, unreadableTask(message, err)
		}
		if len(matches) != 1 || matches[0].ID != id {
			return mail.Message{}, failf("the task %s, which %s leads to, is no longer kept; send a new task with: rewake send %s \"...\"", id, message.ID, target.Name)
		}
		current = matches[0]
	}
	return current, nil
}

// beforeAddendumLock lets a test land a report between the first look at a
// task and the one under the lock.
var beforeAddendumLock = func() {}

// underAddendumLock runs write — the addendum, or an edit of one — under the
// recipient's mailbox lock, once the task it adds to has been asked again
// whether a report on it can still come. The first look, taken without the
// lock, gives a quick refusal; this one is the answer. The turn end that
// writes a report holds the same lock (internal/core/mail/turn_end.go), so a
// report lands either before this look, which then refuses, or after the
// addendum is in the mailbox — read with the task and settled by that report,
// or read after it and owed on its own.
func underAddendumLock(dir string, self registry.Session, epoch string, addendum mail.Message, target registry.Session, write func() error) error {
	beforeAddendumLock()
	return underMailboxLock(dir, target.Name, func() error {
		root, err := rootOf(dir, self, epoch, addendum, target)
		if err != nil {
			return err
		}
		if err := addendumRefusal(dir, self, epoch, root, target); err != nil {
			return err
		}
		return write()
	})
}

// writeSent puts a new message into its recipient's mailbox; an addendum goes
// in under the lock, with its task asked again.
func writeSent(dir string, self registry.Session, epoch string, message mail.Message, target registry.Session) error {
	write := func() error { return mail.Put(dir, message) }
	var err error
	if message.AddendumTo == "" {
		err = write()
	} else {
		err = underAddendumLock(dir, self, epoch, message, target, write)
	}
	var refused *FailedError
	if err != nil && !errors.As(err, &refused) {
		return failf("could not write the message into the mailbox of %s: %v", target.Name, err)
	}
	return err
}

// addendumRefusal says why nothing more can be added to a task, or nil when
// a report on it can still come. rewake edit asks it again of an addendum it
// replaces: the task may have been reported on since the addendum was sent.
func addendumRefusal(dir string, self registry.Session, epoch string, root mail.Message, target registry.Session) error {
	next := fmt.Sprintf("rewake send %s \"...\"", target.Name)
	switch {
	case root.To != target.Name:
		return failf("%s went to %s, not %s; add to it with: rewake send %s \"...\" --to %s", root.ID, root.To, target.Name, root.To, shortRef(root.ID))
	case root.Withdrawn != nil:
		return failf("your %s %s was withdrawn; send a new task with: %s", root.Withdrawn.Kind, root.ID, next)
	case !mail.AsksForWork(root):
		return failf("%s is a %s, which owes nothing to add to; send another with: %s --notify", root.ID, mail.KindOf(root), next)
	}
	item, settled, err := mail.AwaitedOne(dir, self.Name, epoch, root, func(recipient, run string) mail.RecipientRun {
		live, err := registry.LookupReadOnly(dir, recipient)
		switch {
		case err == nil && live.Epoch() == run:
			return mail.RunLive
		case err == nil || !errors.Is(err, registry.ErrNotFound):
			return mail.RunReplaced
		}
		return mail.RunEnded
	})
	switch {
	case err != nil:
		return unreadableTask(root, err)
	case settled:
		return failf("%s has already reported on your %s %s; send the addition as a new task: %s", target.Name, mail.KindOf(root), root.ID, next)
	case item.Gone() || root.ToEpoch != target.Epoch():
		return failf("the session run your %s %s was written for has ended, so no report on it is coming; send a new task with: %s", mail.KindOf(root), root.ID, next)
	case item.Stage == mail.StageFailed:
		return failf("your %s %s was not delivered to %s; send the task again, with the addition, as: %s", mail.KindOf(root), root.ID, target.Name, next)
	}
	return nil
}

// unreadableTask refuses an addition while the task it would add to, or where
// that task stands, cannot be read: the task may have been answered or
// replaced, and an addition on a guess could owe a second report.
func unreadableTask(task mail.Message, err error) error {
	return failf("could not read where your %s %s stands (%v), so nothing was added to it; run the same command again", mail.KindOf(task), task.ID, err)
}
