package inbox

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

func TestGroupedPreparationRechecksAnswerLeasesExpiryAndEpoch(t *testing.T) {
	dir := stateDir(t)
	s := batchServer(dir)
	members := mixedPending(t, dir)
	reserved := message("reserved answer")
	reserved.Kind, reserved.ToEpoch, reserved.InReplyTo = Finished, s.Epoch, []string{"question-id"}
	expired := message("expired")
	expired.ToEpoch, expired.CreatedAt = s.Epoch, time.Now().Add(-2*defaultTTL)
	foreign := message("foreign")
	foreign.ToEpoch = "other-epoch"
	for _, m := range []Message{reserved, expired, foreign} {
		if err := Put(dir, m); err != nil {
			t.Fatal(err)
		}
	}
	var release func()
	var group Message
	r := &reservationFixture{prepare: func(fn func(string) error) error { return fn("root") }, deliver: func(_ context.Context, m Message) Result { group = m; return Result{State: Delivered} }}
	s.Reserve = func(context.Context, Message) (Reservation, error) {
		var err error
		release, err = ReserveAnswer(dir, "api", "question-id")
		return r, err
	}
	s.drain(context.Background())
	defer release()
	if len(group.Batch) != len(members) {
		t.Fatal("reserved, expired or foreign letter entered group")
	}
	available, _ := AvailableUnread(dir, "api", s.Epoch)
	if len(available) != 4 {
		t.Fatal("lease did not protect ordinary reads")
	}
	if status, _ := ReadStatus(dir, "api", expired.ID); status.State != Failed {
		t.Fatal("grouping postponed expiry")
	}
	if _, ok := ReadStatus(dir, "api", foreign.ID); ok {
		t.Fatal("foreign epoch mutated")
	}
	if _, ok := ReadStatus(dir, "api", reserved.ID); ok {
		t.Fatal("reserved answer was announced")
	}
}

func TestCollectionWindowIsFixedAndCancellationDoesNotDropReports(t *testing.T) {
	dir := stateDir(t)
	s := batchServer(dir)
	started, delivered, done := make(chan struct{}), make(chan Message, 8), make(chan struct{})
	s.Ready = func() { close(started) }
	s.Deliver = func(_ context.Context, m Message) Result { delivered <- m; return Result{State: Delivered} }
	ctx, cancel := context.WithCancel(context.Background())
	go func() { defer close(done); s.Serve(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	<-started
	began := time.Now()
	first := message("first")
	first.ToEpoch = s.Epoch
	if err := Put(dir, first); err != nil {
		t.Fatal(err)
	}
	// Arrivals keep coming beyond one window. They must not slide its deadline.
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(4 * collectionInterval)
	defer deadline.Stop()
	count := 1
	for {
		select {
		case <-ticker.C:
			m := message("arriving")
			m.ToEpoch = s.Epoch
			if err := Put(dir, m); err != nil {
				t.Fatal(err)
			}
			count++
		case group := <-delivered:
			if count < 2 || len(group.Batch) < 2 || time.Since(began) < collectionInterval/2 {
				t.Fatal("no collection window")
			}
			return
		case <-deadline.C:
			t.Fatal("arrivals extended the collection deadline")
		}
	}
}

func TestCancellationBeforeGroupAdmissionRetainsOnlyUnexpiredReports(t *testing.T) {
	dir := stateDir(t)
	s := batchServer(dir)
	members := mixedPending(t, dir)
	started, done := make(chan struct{}), make(chan struct{})
	s.Ready = func() { close(started) }
	s.Deliver = func(context.Context, Message) Result {
		t.Error("canceled group announced")
		return Result{State: Delivered}
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { defer close(done); s.Serve(ctx) }()
	<-started
	cancel()
	<-done
	for _, m := range members {
		status, ok := ReadStatus(dir, "api", m.ID)
		if !ok || status.State != Failed || status.ReportAvailable != IsReport(m) {
			t.Fatalf("shutdown outcome %+v", status)
		}
	}
	unread, _ := AvailableUnread(dir, "api", s.Epoch)
	if len(unread) != 1 || unread[0].Kind != Finished {
		t.Fatal("shutdown discarded report")
	}
}

func TestGroupRemembersEveryACKWhenStatusWritesFail(t *testing.T) {
	dir := stateDir(t)
	s := batchServer(dir)
	members := mixedPending(t, dir)
	calls := 0
	for _, m := range members {
		if err := os.Mkdir(statusPath(dir, "api", m.ID), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	s.Deliver = func(context.Context, Message) Result { calls++; return Result{State: Delivered} }
	s.drain(context.Background())
	for _, m := range members {
		if err := os.Remove(statusPath(dir, "api", m.ID)); err != nil {
			t.Fatal(err)
		}
	}
	s.attempts = map[string]time.Time{}
	s.drain(context.Background())
	s.refuseWaiting("session ended")
	if calls != 1 {
		t.Fatal("partial receipt write retried a native group")
	}
	for _, m := range members {
		status, ok := ReadStatus(dir, "api", m.ID)
		if !ok || status.State != Delivered {
			t.Fatalf("lost accepted member: %+v", status)
		}
	}
}

func TestGroupMailboxWaitUsesAdmissionDeadline(t *testing.T) {
	dir := stateDir(t)
	s := batchServer(dir)
	mixedPending(t, dir)
	held, release, released := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(released)
		_ = state.WithMailboxLock(context.Background(), dir, "api", func() error { close(held); <-release; return nil })
	}()
	<-held
	defer func() { close(release); <-released }()
	s.Deliver = func(context.Context, Message) Result {
		t.Error("delivered through locked mailbox")
		return Result{State: Delivered}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); s.drain(ctx) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("mailbox wait ignored admission deadline")
	}
}
