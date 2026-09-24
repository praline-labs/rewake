package inbox

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

type reservationFixture struct {
	prepare func(func(string) error) error
	deliver Deliverer
	closed  bool
}

func (r *reservationFixture) Prepare(fn func(string) error) error           { return r.prepare(fn) }
func (r *reservationFixture) Deliver(ctx context.Context, m Message) Result { return r.deliver(ctx, m) }
func (r *reservationFixture) Close()                                        { r.closed = true }

func TestReservationPrecedesReadabilityAndSurvivesFastRead(t *testing.T) {
	dir := stateDir(t)
	task := message("reserved-task")
	task.FromEpoch = "2.2"
	task.ToEpoch = "1.1"
	if err := Put(dir, task); err != nil {
		t.Fatal(err)
	}
	r := &reservationFixture{}
	s := &Server{Dir: dir, Name: "api", Epoch: "1.1", attempts: map[string]time.Time{}, outcomes: map[string]Result{}}
	s.Reserve = func(ctx context.Context, _ Message) (Reservation, error) {
		unread, err := PeekUnread(dir, "api", "1.1")
		if err != nil || len(unread) != 0 {
			t.Fatalf("readable before reservation: %v %v", unread, err)
		}
		bounded, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		if err := state.WithMailboxLock(bounded, dir, "api", func() error { return nil }); err != nil {
			t.Fatalf("native readiness waited under mailbox lock: %v", err)
		}
		return r, nil
	}
	r.prepare = func(fn func(string) error) error { return fn("A") }
	r.deliver = func(_ context.Context, m Message) Result {
		if r.closed || m.DeliveryThread != "A" || deliveryThread(dir, "api", m.ID) != "A" {
			t.Fatal("reservation/context lost before ACK")
		}
		if err := state.WithMailboxLock(context.Background(), dir, "api", func() error { return MarkRead(dir, "api", "1.1", m, true) }); err != nil {
			t.Fatal(err)
		}
		return Result{State: Failed, Detail: "notice acknowledgement lost"}
	}
	s.drain(context.Background())
	status, _ := ReadStatus(dir, "api", task.ID)
	if !r.closed || status.State != Read || !ReportThreadChanged(dir, "api", []string{task.ID}, "B") {
		t.Fatalf("fast read or generation context lost: %+v", status)
	}
}

func TestReservationRefusalKeepsOnlyReportsReadable(t *testing.T) {
	for _, kind := range []Kind{Task, Question, Note, Finished, Error, Stopped} {
		t.Run(string(kind), func(t *testing.T) {
			dir := stateDir(t)
			m := message("refused")
			m.Kind = kind
			if err := Put(dir, m); err != nil {
				t.Fatal(err)
			}
			s := &Server{Dir: dir, Name: "api", attempts: map[string]time.Time{}, outcomes: map[string]Result{}, Reserve: func(context.Context, Message) (Reservation, error) { return nil, errors.New("no accepted destination") }, Deliver: func(context.Context, Message) Result { t.Fatal("sent after refusal"); return Result{} }}
			s.drain(context.Background())
			status, _ := ReadStatus(dir, "api", m.ID)
			unread, _ := PeekUnread(dir, "api", "")
			if status.State != Failed || status.ReportAvailable != IsReport(m) || (len(unread) == 1) != IsReport(m) {
				t.Fatalf("refusal lost report or exposed task: %+v unread=%d", status, len(unread))
			}
			s.drain(context.Background())
			if unread, _ := PeekUnread(dir, "api", ""); (len(unread) == 1) != IsReport(m) {
				t.Fatal("recovery changed readability")
			}
		})
	}
}

func TestReservationIsReleasedOnReadabilityFailure(t *testing.T) {
	dir := stateDir(t)
	m := message("expired-reservation")
	if err := Put(dir, m); err != nil {
		t.Fatal(err)
	}
	r := &reservationFixture{prepare: func(func(string) error) error { return ErrThreadUnavailable }, deliver: func(context.Context, Message) Result { t.Fatal("invalid admission sent work"); return Result{} }}
	s := &Server{Dir: dir, Name: "api", attempts: map[string]time.Time{}, outcomes: map[string]Result{}, Reserve: func(context.Context, Message) (Reservation, error) { return r, nil }}
	s.drain(context.Background())
	unread, _ := PeekUnread(dir, "api", "")
	if !r.closed || len(unread) != 0 {
		t.Fatal("invalid reservation exposed mail or held native gate")
	}
}

func TestReportStaysReadableWhenReservedDestinationDisappears(t *testing.T) {
	dir := stateDir(t)
	m := message("lost-binding")
	m.Kind = Finished
	if err := Put(dir, m); err != nil {
		t.Fatal(err)
	}
	r := &reservationFixture{prepare: func(func(string) error) error { return ErrThreadUnavailable }, deliver: func(context.Context, Message) Result { t.Fatal("lost binding delivered"); return Result{} }}
	s := &Server{Dir: dir, Name: "api", attempts: map[string]time.Time{}, outcomes: map[string]Result{}, Reserve: func(context.Context, Message) (Reservation, error) { return r, nil }}
	s.drain(context.Background())
	unread, _ := PeekUnread(dir, "api", "")
	status, _ := ReadStatus(dir, "api", m.ID)
	if !r.closed || len(unread) != 1 || !status.ReportAvailable || status.State != Failed {
		t.Fatalf("lost-binding report was discarded: %+v", status)
	}
}

// A destination that cannot take the message yet — a conversation being
// compacted — keeps it pending and unread, and the next attempt delivers it.
func TestReservationNotYetKeepsTheMessagePending(t *testing.T) {
	dir := stateDir(t)
	m := message("after-compaction")
	if err := Put(dir, m); err != nil {
		t.Fatal(err)
	}
	busy := true
	r := &reservationFixture{prepare: func(fn func(string) error) error { return fn("A") }, deliver: func(context.Context, Message) Result { return Result{State: Delivered} }}
	s := &Server{Dir: dir, Name: "api", attempts: map[string]time.Time{}, outcomes: map[string]Result{}, Reserve: func(context.Context, Message) (Reservation, error) {
		if busy {
			return nil, fmt.Errorf("%w: a compaction is running", ErrNotYet)
		}
		return r, nil
	}}
	s.drain(context.Background())
	status, _ := ReadStatus(dir, "api", m.ID)
	unread, _ := PeekUnread(dir, "api", "")
	if status.State != Pending || len(unread) != 0 {
		t.Fatalf("while compacting: %+v, %d unread", status, len(unread))
	}
	busy = false
	s.attempts[m.ID] = time.Now().Add(-retryInterval)
	s.drain(context.Background())
	if status, _ := ReadStatus(dir, "api", m.ID); status.State != Delivered {
		t.Fatalf("after the compaction: %+v", status)
	}
}
