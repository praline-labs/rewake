package inbox

import (
	"context"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

func TestNewBatchPreservesReservedAnswersAndExpiry(t *testing.T) {
	dir := stateDir(t)
	s := batchServer(dir)
	var announced []Message
	s.Deliver = func(_ context.Context, m Message) Result {
		announced = append(announced, m)
		return Result{State: Delivered}
	}
	first := message("announced")
	first.ToEpoch = s.Epoch
	if err := Put(dir, first); err != nil {
		t.Fatal(err)
	}
	s.drain(context.Background())
	later := mixedPending(t, dir)
	expired := message("expired grant")
	expired.ToEpoch, expired.Kind, expired.GrantGit, expired.CreatedAt = s.Epoch, Task, true, time.Now().Add(-2*DefaultTTL)
	answer := message("reserved report")
	answer.ToEpoch, answer.Kind, answer.InReplyTo = s.Epoch, Finished, []string{"question"}
	foreign := message("other epoch")
	foreign.ToEpoch = "other"
	for _, m := range []Message{expired, answer, foreign} {
		if err := Put(dir, m); err != nil {
			t.Fatal(err)
		}
	}
	release, err := ReserveAnswer(dir, s.Name, "question")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	s.drain(context.Background())
	if len(announced) != 2 {
		t.Fatal("new eligible mail was not dispatched")
	}
	if available, _ := AvailableUnread(dir, s.Name, s.Epoch); len(available) != 1+len(later) {
		t.Fatal("reserved answer became ordinary unread or new mail was lost")
	}
	if st, _, _ := ReadStatus(dir, s.Name, expired.ID); st.State != Failed {
		t.Fatal("outstanding wake postponed expiry")
	}
	if _, known, _ := ReadStatus(dir, s.Name, foreign.ID); known {
		t.Fatal("foreign epoch changed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, found, err := AwaitAnswer(ctx, dir, s.Name, s.Epoch, "question", func(m Message) error {
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

func (r *consumedAnnouncement) DeliverChecked(_ context.Context, _ Message, valid func() bool) Result {
	r.consume()
	if !valid() {
		return Result{State: Pending, Detail: "announcement membership changed before send"}
	}
	*r.calls++
	return Result{State: Delivered}
}

func TestConsumedDuringPreparationNeverProducesAStaleAnnouncement(t *testing.T) {
	dir := stateDir(t)
	s := batchServer(dir)
	members := mixedPending(t, dir)
	calls := 0
	s.Reserve = func(context.Context, Message) (Reservation, error) {
		return &consumedAnnouncement{reservationFixture: &reservationFixture{prepare: func(fn func(string) error) error { return fn("A") }}, calls: &calls, consume: func() {
			if err := state.WithMailboxLock(context.Background(), dir, s.Name, func() error {
				for _, m := range members {
					if err := MarkRead(dir, s.Name, s.Epoch, m, false); err != nil {
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
		if st, _, _ := ReadStatus(dir, s.Name, m.ID); st.State != Read {
			t.Fatal("read outcome was undone")
		}
	}
}
