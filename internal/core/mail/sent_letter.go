package mail

import (
	"context"
	"errors"
	"fmt"

	"github.com/praline-labs/rewake/internal/core/registry"
)

// A letter this run sent can still be acted on while nobody has read it: it
// can be withdrawn, replaced by an edit, or added to by an addendum
// (docs/delivery-sent.md). Each act starts from the letter the caller's
// reference named and follows the edits since to the letter that stands now.
// Then it takes the recipient's mailbox lock, asks again under it whatever
// decides the act, and writes. A turn end that writes a report holds the
// same lock (turn_end.go), so a report lands either before the look that
// decides, which then refuses, or after the write.

// Reason says why an act on a sent letter was refused.
type Reason int

const (
	// ReplacementLost is a letter an edit replaced whose replacement, Leads,
	// is no longer kept.
	ReplacementLost Reason = iota + 1
	// TaskLost is a task, Leads, that the letter leads to and that is no
	// longer kept.
	TaskLost
	// TaskUnknown is a letter whose task, or where that task stands, cannot
	// be read (Err): the task may have been answered or replaced, and an
	// addition made on a guess could owe a second report.
	TaskUnknown
	// OtherRecipient is a task that went to another recipient than the one
	// named.
	OtherRecipient
	// TaskWithdrawn is a task that was withdrawn.
	TaskWithdrawn
	// OwesNothing is a note, which owes nothing to add to.
	OwesNothing
	// Reported is a task its recipient has already reported on.
	Reported
	// RecipientGone is a task whose recipient run has ended, so no report on it is
	// coming.
	RecipientGone
	// NotDelivered is a task that was not delivered.
	NotDelivered
	// EditOfWithdrawn is a letter to replace that was withdrawn outright.
	EditOfWithdrawn
)

var reasonText = map[Reason]string{
	ReplacementLost: "its replacement is no longer kept",
	TaskLost:        "the task it leads to is no longer kept",
	TaskUnknown:     "where its task stands cannot be read",
	OtherRecipient:  "it went to another recipient",
	TaskWithdrawn:   "it was withdrawn",
	OwesNothing:     "it owes nothing to add to",
	Reported:        "it was already reported on",
	RecipientGone:   "the run it was written for has ended",
	NotDelivered:    "it was not delivered",
	EditOfWithdrawn: "it was already withdrawn",
}

// Refusal is an act on a sent letter that a check refused. Nothing was
// written.
type Refusal struct {
	Reason Reason
	// Letter is the letter the reason is about.
	Letter Message
	// Leads is the id the letter led to, for ReplacementLost and TaskLost.
	Leads string
	// Err is what could not be read, for TaskUnknown.
	Err error
}

func (r *Refusal) Error() string {
	text := fmt.Sprintf("%s: %s", r.Letter.ID, reasonText[r.Reason])
	if r.Err != nil {
		text += fmt.Sprintf(" (%v)", r.Err)
	}
	return text
}

func (r *Refusal) Unwrap() error { return r.Err }

// CurrentSent is the letter that stands now for one this run sent: through
// every edit since, what replaces it, and otherwise the letter itself as it
// is on disk. After an edit the old id is still the sender's name for the
// task, so an act by it acts on the task as it stands. The record is read
// afresh rather than trusted, since the caller's copy may predate an edit
// that ran beside it. Without the lock this is a first look; under it, the
// look that decides. A chain that cannot be read is StepChain; a replacement
// no longer kept is refused as ReplacementLost. On a failure the letter given
// is answered.
func (b Box) CurrentSent(letter Message) (Message, error) {
	id, err := currentTask(b.dir, letter.To, letter.ID)
	if err != nil {
		return letter, failedAt(StepChain, err)
	}
	matches, err := b.SentMatching(id)
	if err != nil {
		return letter, failedAt(StepChain, err)
	}
	if len(matches) == 1 && matches[0].ID == id {
		return matches[0], nil
	}
	if id == letter.ID {
		return letter, nil
	}
	return letter, &Refusal{Reason: ReplacementLost, Letter: letter, Leads: id}
}

// TaskToAddTo is the task an addition to letter adds to, refused where no
// report on it can come. An addendum to an addendum adds to the task they
// both belong to, and a task an edit replaced is followed to its
// replacement. Taken without the lock, it gives a quick refusal; Send and
// Replace ask it again under the lock, and that look is the answer.
func (b Box) TaskToAddTo(letter Message, target registry.Session) (Message, error) {
	root, err := b.rootOf(letter)
	if err != nil {
		return Message{}, err
	}
	return root, b.refuseAddition(root, target)
}

// rootOf follows at most two steps: from an edited addendum to its
// replacement, then to its task.
func (b Box) rootOf(letter Message) (Message, error) {
	current := letter
	for range 2 {
		id := current.ID
		if current.AddendumTo != "" {
			id = current.AddendumTo
		}
		id, err := currentTask(b.dir, current.To, id)
		if err != nil {
			return Message{}, &Refusal{Reason: TaskUnknown, Letter: letter, Err: err}
		}
		if id == current.ID {
			return current, nil
		}
		matches, err := b.SentMatching(id)
		if err != nil {
			return Message{}, &Refusal{Reason: TaskUnknown, Letter: letter, Err: err}
		}
		if len(matches) != 1 || matches[0].ID != id {
			return Message{}, &Refusal{Reason: TaskLost, Letter: letter, Leads: id}
		}
		current = matches[0]
	}
	return current, nil
}

// refuseAddition says why nothing more can be added to a task, or nil when a
// report on it can still come.
func (b Box) refuseAddition(root Message, target registry.Session) error {
	refuse := func(reason Reason) error { return &Refusal{Reason: reason, Letter: root} }
	switch {
	case root.To != target.Name:
		return refuse(OtherRecipient)
	case root.Withdrawn != nil:
		return refuse(TaskWithdrawn)
	case !AsksForWork(root):
		return refuse(OwesNothing)
	}
	item, settled, err := b.StandingOf(root)
	switch {
	case err != nil:
		return &Refusal{Reason: TaskUnknown, Letter: root, Err: err}
	case settled:
		return refuse(Reported)
	case item.Gone() || root.ToEpoch != target.Epoch():
		return refuse(RecipientGone)
	case item.Stage == StageFailed:
		return refuse(NotDelivered)
	}
	return nil
}

// underRecipientLock runs fn under the mailbox lock of recipient, waiting for
// it as long as a reader does. A refusal or a step's failure is answered as it
// is; a lock that could not be taken is StepLock.
func (b Box) underRecipientLock(parent context.Context, recipient string, fn func() error) error {
	ctx, cancel := context.WithTimeout(parent, ReadLockWait)
	defer cancel()
	err := Open(b.dir, recipient, "").WithLock(ctx, func(Locked) error { return fn() })
	var refusal *Refusal
	if errors.As(err, &refusal) {
		return err
	}
	return lockFailure(err)
}
