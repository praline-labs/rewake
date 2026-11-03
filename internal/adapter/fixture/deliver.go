//go:build rewaketest

package fixture

import (
	"context"
	"errors"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
)

// deliverBound is how long the program has to take a notice.
const deliverBound = 5 * time.Second

// Deliver hands one notice to the program, into the conversation it holds.
// Without a live Wake it fails: the adapter delivers only through what the
// program proved.
func (b *backend) Deliver(_ context.Context, message mail.Message) mail.Result {
	b.mu.Lock()
	l, live, thread := b.link, b.live[Wake], b.thread
	b.mu.Unlock()
	if l == nil || !live {
		return mail.Result{State: mail.Failed, Detail: "the fixture has no live wake"}
	}
	if message.DeliveryThread != "" && message.DeliveryThread != thread {
		return mail.Result{State: mail.Failed, Detail: mail.ErrThreadUnavailable.Error()}
	}
	answer, err := l.ask(Frame{Op: opDeliver, Thread: thread, Letter: message.ID, Notice: mail.Notice(message), Members: members(message)}, deliverBound)
	if errors.Is(err, errClosed) {
		return mail.Result{State: mail.Pending, Detail: err.Error()}
	}
	if err != nil {
		return mail.Result{State: mail.Failed, Detail: err.Error() + "; delivery was not retried automatically"}
	}
	if !answer.OK {
		return mail.Result{State: mail.Failed, Detail: "the fixture refused the notice: " + answer.Error}
	}
	via := "fixture"
	if answer.Steered {
		via = "fixture, steered into the running turn"
	}
	return mail.Result{State: mail.Delivered, Via: via}
}

func members(message mail.Message) []Member {
	letters := message.Batch
	if len(letters) == 0 {
		letters = []mail.Message{message}
	}
	out := make([]Member, 0, len(letters))
	for _, letter := range letters {
		member := Member{ID: letter.ID, From: letter.From, FromEpoch: letter.FromEpoch, To: letter.To, ToEpoch: letter.ToEpoch, Replaces: letter.Replaces}
		if letter.Recall != nil {
			member.Recalls = letter.Recall.ID
		}
		out = append(out, member)
	}
	return out
}
