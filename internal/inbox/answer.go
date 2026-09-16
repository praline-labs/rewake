package inbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

// A question blocks its sender until the receiver's turn ends. The answer is
// the receiver's report, and it is handed to the waiting send rather than
// announced to the agent: the agent is already waiting for it in a tool call,
// and a second path would show it twice. The send keeps a mark fresh while it
// waits; a mark that stopped being touched belongs to a send that is gone.

// answeringFresh is how recently a waiting send must have touched its mark to
// count as waiting.
const answeringFresh = 3 * time.Second

// answerPoll is how often a waiting send looks for its answer and touches its
// mark. A report takes a turn to arrive; a second is fine.
const answerPoll = time.Second

// awaitedHere reports whether a send in this session waits for this report now.
// The caller holds the mailbox lock.
func awaitedHere(dir, name string, message Message) bool {
	if KindOf(message) != Finished {
		return false
	}
	for _, id := range message.InReplyTo {
		info, err := os.Stat(filepath.Join(state.AnsweringPath(dir, name), id))
		if err == nil && time.Since(info.ModTime()) < answeringFresh {
			return true
		}
	}
	return false
}

// AwaitAnswer shows the matching report under the mailbox lock, then records
// receipt. ReserveAnswer must precede publishing the question and its release
// must be deferred by the caller through the entire delivery and output path.
func AwaitAnswer(ctx context.Context, dir, name, epoch, question string, show func(Message) error) (Message, bool, error) {
	for {
		answer, found, err := takeAnswer(ctx, dir, name, epoch, question, show)
		if err != nil || found {
			return answer, found, err
		}
		select {
		case <-ctx.Done():
			beforeGivingUp()
			last, cancel := context.WithTimeout(context.Background(), 2*answeringFresh)
			defer cancel()
			return takeAnswer(last, dir, name, epoch, question, show)
		case <-time.After(answerPoll):
		}
	}
}

// beforeGivingUp lets a test land an answer immediately before the final look.
var beforeGivingUp = func() {}

func takeAnswer(ctx context.Context, dir, name, epoch, question string, show func(Message) error) (Message, bool, error) {
	var answer Message
	found := false
	err := state.WithMailboxLock(ctx, dir, name, func() error {
		messages, err := PeekUnread(dir, name, epoch)
		if err != nil {
			return err
		}
		for _, message := range messages {
			if !answers(message, question) {
				continue
			}
			if err := show(message); err != nil {
				return err
			}
			if err := receiveAnswer(dir, name, epoch, question, message); err != nil {
				return err
			}
			answer, found = message, true
			return nil
		}
		return nil
	})
	if errors.Is(err, state.ErrMailboxBusy) {
		return Message{}, false, nil
	}
	return answer, found, err
}

// answers reports whether a message is the report that settles a question.
func answers(message Message, question string) bool {
	if KindOf(message) != Finished {
		return false
	}
	for _, id := range message.InReplyTo {
		if id == question {
			return true
		}
	}
	return false
}

// removeMark releases a reservation after receipt or after the command ends.
func removeMark(dir, name, mark string) {
	_ = os.Remove(mark)
	_ = state.SyncDir(state.AnsweringPath(dir, name))
}
