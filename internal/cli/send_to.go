package cli

import (
	"errors"
	"fmt"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
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
	if kind.kind != inbox.Task {
		return "", &UsageError{Command: call.Command, Message: "--to adds to a task already sent and is a task itself; it excludes " + kind.flag.Flag + "."}
	}
	if _, grant := call.Flags["grant-git"]; grant {
		return "", &UsageError{Command: call.Command, Message: "--to excludes --grant-git: an addendum carries no grant of its own; send a new task with --grant-git instead."}
	}
	self, epoch, root, err := sentBySelf(call, dir, reference, fmt.Sprintf("rewake send %s \"...\" --to <id>", target.Name))
	if err != nil {
		return "", err
	}
	if root.AddendumTo != "" {
		// An addendum to an addendum adds to the task they both belong to.
		if root, err = rootOf(dir, self, epoch, root, target); err != nil {
			return "", err
		}
	}
	if err := addendumRefusal(dir, self, epoch, root, target); err != nil {
		return "", err
	}
	return root.ID, nil
}

// rootOf is the task an addendum adds to.
func rootOf(dir string, self registry.Session, epoch string, addendum inbox.Message, target registry.Session) (inbox.Message, error) {
	matches := inbox.SentMatching(dir, self.Name, epoch, addendum.AddendumTo)
	if len(matches) != 1 {
		return inbox.Message{}, failf("the task %s that %s adds to is no longer kept; send a new task with: rewake send %s \"...\"", addendum.AddendumTo, addendum.ID, target.Name)
	}
	return matches[0], nil
}

// addendumRefusal says why nothing more can be added to a task, or nil when
// a report on it can still come. rewake edit asks it again of an addendum it
// replaces: the task may have been reported on since the addendum was sent.
func addendumRefusal(dir string, self registry.Session, epoch string, root inbox.Message, target registry.Session) error {
	next := fmt.Sprintf("rewake send %s \"...\"", target.Name)
	switch {
	case root.To != target.Name:
		return failf("%s went to %s, not %s; add to it with: rewake send %s \"...\" --to %s", root.ID, root.To, target.Name, root.To, shortRef(root.ID))
	case root.Withdrawn != nil:
		return failf("your %s %s was withdrawn; send a new task with: %s", root.Withdrawn.Kind, root.ID, next)
	case !inbox.AsksForWork(root):
		return failf("%s is a %s, which owes nothing to add to; send another with: %s --notify", root.ID, inbox.KindOf(root), next)
	}
	item, settled := inbox.AwaitedOne(dir, self.Name, epoch, root, func(recipient, run string) inbox.RecipientRun {
		live, err := registry.LookupReadOnly(dir, recipient)
		switch {
		case err == nil && live.Epoch() == run:
			return inbox.RunLive
		case err == nil || !errors.Is(err, registry.ErrNotFound):
			return inbox.RunReplaced
		}
		return inbox.RunEnded
	})
	switch {
	case settled:
		return failf("%s has already reported on your %s %s; send the addition as a new task: %s", target.Name, inbox.KindOf(root), root.ID, next)
	case item.Gone() || root.ToEpoch != target.Epoch():
		return failf("the session run your %s %s was written for has ended, so no report on it is coming; send a new task with: %s", inbox.KindOf(root), root.ID, next)
	case item.Stage == inbox.StageFailed:
		return failf("your %s %s was not delivered to %s; send the task again, with the addition, as: %s", inbox.KindOf(root), root.ID, target.Name, next)
	}
	return nil
}
