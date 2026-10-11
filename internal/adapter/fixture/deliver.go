//go:build rewakefixture

package fixture

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	mail "github.com/praline-labs/rewake/internal/inbox"
)

// deliverBound is how long the program has to take a notice or a
// reservation.
const deliverBound = 5 * time.Second

// Deliver hands one notice to the program. Without a live Wake it fails: the
// adapter delivers only through what the program proved.
func (b *backend) Deliver(ctx context.Context, message mail.Message) mail.Result {
	reserved, err := b.Reserve(ctx, message)
	if errors.Is(err, mail.ErrNotYet) {
		return mail.Result{State: mail.Pending, Detail: err.Error()}
	}
	if err != nil {
		return mail.Result{State: mail.Failed, Detail: err.Error()}
	}
	defer reserved.Close()
	if message.DeliveryThread != "" {
		if err := reserved.Prepare(func(thread string) error {
			if thread != message.DeliveryThread {
				return mail.ErrThreadUnavailable
			}
			return nil
		}); err != nil {
			return mail.Result{State: mail.Failed, Detail: err.Error()}
		}
	}
	return reserved.Deliver(ctx, message)
}

// Reserve asks the program to hold its conversation for one notice, which it
// answers before any letter of the notice is readable — the order the batch
// path needs from a harness's turn start.
func (b *backend) Reserve(_ context.Context, _ mail.Message) (mail.Reservation, error) {
	l, live := b.isLive(Wake)
	if !live {
		return nil, fmt.Errorf("%w: the fixture has no live wake", mail.ErrThreadUnavailable)
	}
	answer, err := l.ask(Frame{Op: opReserve}, deliverBound)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", mail.ErrNotYet, err)
	}
	if !answer.OK || answer.Thread == "" {
		return nil, fmt.Errorf("%w: the fixture refused the reservation: %s", mail.ErrNotYet, answer.Error)
	}
	return &reservation{backend: b, link: l, thread: answer.Thread}, nil
}

// reservation is one notice's hold on the program's conversation.
type reservation struct {
	backend *backend
	link    *link
	thread  string
	once    sync.Once
}

// Prepare runs fn with the reserved conversation while the reservation's
// connection holds.
func (r *reservation) Prepare(fn func(string) error) error {
	select {
	case <-r.link.closed:
		return fmt.Errorf("%w: the fixture's connection closed", mail.ErrNotYet)
	default:
	}
	return fn(r.thread)
}

func (r *reservation) Deliver(_ context.Context, message mail.Message) mail.Result {
	if l, live := r.backend.isLive(Wake); !live || l != r.link {
		return mail.Result{State: mail.Failed, Detail: "the fixture's wake was withdrawn"}
	}
	answer, err := r.link.ask(Frame{Op: opDeliver, Thread: r.thread, Letter: message.ID, Notice: mail.Notice(message), Members: members(message)}, deliverBound)
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

// Close lets the program's conversation go.
func (r *reservation) Close() {
	r.once.Do(func() { _ = r.link.send(Frame{Op: opRelease, Thread: r.thread}) })
}

func members(message mail.Message) []Member {
	letters := message.Batch
	if len(letters) == 0 {
		letters = []mail.Message{message}
	}
	out := make([]Member, 0, len(letters))
	for _, letter := range letters {
		member := Member{ID: letter.ID, From: letter.From, FromEpoch: letter.FromEpoch, To: letter.To, ToEpoch: letter.ToEpoch, Replaces: letter.Replaces, Grants: letter.GrantDirs}
		if letter.Recall != nil {
			member.Recalls = letter.Recall.ID
		}
		out = append(out, member)
	}
	return out
}
