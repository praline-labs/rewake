package mail

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ReadLockWait is how long a reader waits for another reader, or for the
// server, to let go of the mailbox.
const ReadLockWait = 10 * time.Second

// ReadRequest is what one read of a mailbox asks for.
type ReadRequest struct {
	// Selected is the id of the one letter to read; empty reads every
	// available one.
	Selected string
	// Peek shows the letters and counts none as read.
	Peek bool
	// Reports says the reader's role owes reports on the work it reads.
	Reports bool
}

// Read hands this run the unread mail of its name, every run's: an earlier
// run that ended before reading what was announced to it leaves it in
// unread/, and nobody else will ever read it. A letter held for a question's
// answer is not among them.
//
// The mailbox lock is held from looking to marking, so two readers at once —
// parallel tool calls, a command run again while the first still prints — do
// not both take the same task, and the server records no delivery over a read
// half done. show gets every letter taken, and the letters count as read only
// once it answered nil (docs/rules/effects.md, E4 and E5): one marked first
// and lost on the way would be gone for good; shown twice, it is merely shown
// twice. A task an earlier run never read owes nothing when read now: it is
// lost to its sender, who may have sent it again.
//
// It answers the letters read, or a *StepError naming the step that stopped
// it: StepSelect when the letter asked for is not available, StepShow when
// show failed and nothing was marked, StepMark when a record failed after the
// showing and the letters stay unread to be shown again.
func (b Box) Read(parent context.Context, request ReadRequest, show func([]Message) error) ([]Message, error) {
	ctx, cancel := context.WithTimeout(parent, ReadLockWait)
	defer cancel()
	var read []Message
	err := b.WithLock(ctx, func(Locked) error {
		var err error
		read, err = b.readLocked(request, show)
		return err
	})
	return read, lockFailure(err)
}

// lockFailure is the error of a locked section: a step's own, or the lock's,
// which took none of them.
func lockFailure(err error) error {
	var step *StepError
	if err == nil || errors.As(err, &step) {
		return err
	}
	return failedAt(StepLock, err)
}

func (b Box) readLocked(request ReadRequest, show func([]Message) error) ([]Message, error) {
	messages, err := availableUnread(b.dir, b.name, "")
	if err != nil {
		return nil, failedAt(StepUnread, err)
	}
	if request.Selected != "" {
		messages = selectedOf(messages, request.Selected)
		if messages == nil {
			return nil, failedAt(StepSelect, fmt.Errorf("message %q is not available", request.Selected))
		}
	}
	if err := show(messages); err != nil {
		return nil, failedAt(StepShow, err)
	}
	if request.Peek {
		return messages, nil
	}
	for _, message := range messages {
		reports := request.Reports && message.ToEpoch == b.epoch
		if err := markRead(b.dir, b.name, b.epoch, message, reports); err != nil {
			return nil, failedAt(StepMark, err)
		}
	}
	return messages, nil
}

// selectedOf is the letter of the id among messages, nil when none is.
func selectedOf(messages []Message, id string) []Message {
	for _, message := range messages {
		if message.ID == id {
			return []Message{message}
		}
	}
	return nil
}
