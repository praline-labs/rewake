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

// AwaitAnswer blocks until the report answering a question arrives in this
// run's mailbox, or ctx ends. The answer is marked read and returned. At the end
// the mark goes and the mailbox is looked at once more, under the lock: an
// answer that arrived just then was not announced, so it must be taken here.
func AwaitAnswer(ctx context.Context, dir, name, epoch, question string) (Message, bool, error) {
	marks := state.AnsweringPath(dir, name)
	if err := state.EnsureSubdir(marks); err != nil {
		return Message{}, false, err
	}
	mark := filepath.Join(marks, question)
	if err := os.WriteFile(mark, nil, 0o600); err != nil {
		return Message{}, false, err
	}

	for {
		answer, found, err := takeAnswer(ctx, dir, name, epoch, question, "")
		if err != nil || found {
			removeMark(dir, name, mark)
			return answer, found, err
		}
		select {
		case <-ctx.Done():
			beforeGivingUp()
			return takeAnswerAtTheEnd(dir, name, epoch, question, mark)
		case <-time.After(answerPoll):
			now := time.Now()
			_ = os.Chtimes(mark, now, now)
		}
	}
}

// beforeGivingUp runs when the wait has ended and before the last look. It does
// nothing; a test lands an answer there.
var beforeGivingUp = func() {}

// takeAnswerAtTheEnd removes the mark and takes an answer that got in first.
func takeAnswerAtTheEnd(dir, name, epoch, question, mark string) (Message, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*answeringFresh)
	defer cancel()
	return takeAnswer(ctx, dir, name, epoch, question, mark)
}

// takeAnswer looks for the answer under the lock and marks it read. Given a
// mark, it removes the mark in the same step: this is the last look.
func takeAnswer(ctx context.Context, dir, name, epoch, question, mark string) (Message, bool, error) {
	var answer Message
	found := false
	err := state.WithMailboxLock(ctx, dir, name, func() error {
		if mark != "" {
			_ = os.Remove(mark)
		}
		messages, err := PeekUnread(dir, name, epoch)
		if err != nil {
			return err
		}
		for _, message := range messages {
			if !answers(message, question) {
				continue
			}
			// An answer is a report, and a report owes nothing.
			if err := MarkRead(dir, name, epoch, message, false); err != nil {
				return err
			}
			answer, found = message, true
			return nil
		}
		return nil
	})
	if errors.Is(err, state.ErrMailboxBusy) && mark == "" {
		// Somebody else is reading; the next look will do.
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

// removeMark takes the mark away once the answer is in hand.
func removeMark(dir, name, mark string) {
	_ = os.Remove(mark)
	_ = state.SyncDir(state.AnsweringPath(dir, name))
}
