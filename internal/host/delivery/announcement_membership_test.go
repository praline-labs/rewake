package delivery

import (
	"context"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/state"
)

func TestNewBatchPreservesReservedAnswersAndExpiry(t *testing.T) {
	dir := stateDir(t)
	s := batchServer(dir)
	var announced []mail.Message
	s.Deliver = func(_ context.Context, m mail.Message) mail.Result {
		announced = append(announced, m)
		return mail.Result{State: mail.Delivered}
	}
	first := message("announced")
	first.ToEpoch = s.Epoch
	if err := mail.Put(dir, first); err != nil {
		t.Fatal(err)
	}
	s.drain(context.Background())
	later := mixedPending(t, dir)
	expired := message("expired task")
	expired.ToEpoch, expired.Kind, expired.CreatedAt = s.Epoch, mail.Task, time.Now().Add(-2*DefaultTTL)
	answer := message("reserved report")
	answer.ToEpoch, answer.Kind, answer.InReplyTo = s.Epoch, mail.Finished, []string{"question"}
	foreign := message("other epoch")
	foreign.ToEpoch = "other"
	for _, m := range []mail.Message{expired, answer, foreign} {
		if err := mail.Put(dir, m); err != nil {
			t.Fatal(err)
		}
	}
	release, err := mail.Open(dir, s.Name, "").ReserveAnswer("question")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	s.drain(context.Background())
	if len(announced) != 2 {
		t.Fatal("new eligible mail was not dispatched")
	}
	if available, _ := mail.Open(dir, s.Name, s.Epoch).AvailableUnread(); len(available) != 1+len(later) {
		t.Fatal("reserved answer became ordinary unread or new mail was lost")
	}
	if st, _, _ := mail.ReadStatus(dir, s.Name, expired.ID); st.State != mail.Failed {
		t.Fatal("outstanding wake postponed expiry")
	}
	if _, known, _ := mail.ReadStatus(dir, s.Name, foreign.ID); known {
		t.Fatal("foreign epoch changed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, found, err := mail.Open(dir, s.Name, s.Epoch).AwaitAnswer(ctx, "question", func(m mail.Message) error {
		if m.ID != answer.ID {
			t.Fatal("wrong reserved answer")
		}
		return nil
	}); err != nil || !found {
		t.Fatal("reserved answer was blocked", err)
	}
	s.drain(ctx)
	if len(announced) != 2 || len(announced[1].Batch) != len(later) {
		t.Fatal("expired/read/reserved entries entered next announcement")
	}
	for i, m := range announced[1].Batch {
		if m.ID != later[i].ID {
			t.Fatal("fixed IDs changed")
		}
	}
}

type consumedAnnouncement struct {
	*reservationFixture
	consume func()
	calls   *int
}

func (r *consumedAnnouncement) DeliverChecked(_ context.Context, _ mail.Message, valid func() bool) mail.Result {
	r.consume()
	if !valid() {
		return mail.Result{State: mail.Pending, Detail: "announcement membership changed before send"}
	}
	*r.calls++
	return mail.Result{State: mail.Delivered}
}

func TestConsumedDuringPreparationNeverProducesAStaleAnnouncement(t *testing.T) {
	dir := stateDir(t)
	s := batchServer(dir)
	members := mixedPending(t, dir)
	calls := 0
	s.Reserve = func(context.Context, mail.Message) (mail.Reservation, error) {
		return &consumedAnnouncement{reservationFixture: &reservationFixture{prepare: func(fn func(string) error) error { return fn("A") }}, calls: &calls, consume: func() {
			if err := state.WithMailboxLock(context.Background(), dir, s.Name, func() error {
				for _, m := range members {
					if err := mail.Open(dir, s.Name, s.Epoch).MarkRead(m, false); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		}}, nil
	}
	s.drain(context.Background())
	s.attempts = map[string]time.Time{}
	s.drain(context.Background())
	if calls != 0 {
		t.Fatal("already consumed work produced a stale notice")
	}
	for _, m := range members {
		if st, _, _ := mail.ReadStatus(dir, s.Name, m.ID); st.State != mail.Read {
			t.Fatal("read outcome was undone")
		}
	}
}
