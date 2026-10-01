package inbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

func mixedPending(t *testing.T, dir string) []Message {
	t.Helper()
	var result []Message
	for _, kind := range []Kind{Task, Question, Note, Finished} {
		m := message(string(kind) + " preview\nfull body")
		m.Kind, m.FromEpoch, m.ToEpoch = kind, "sender-epoch", "receiver-epoch"
		if err := Put(dir, m); err != nil {
			t.Fatal(err)
		}
		result = append(result, m)
	}
	return result
}

func batchServer(dir string) *Server {
	return &Server{Dir: dir, Name: "api", Epoch: "receiver-epoch", attempts: map[string]time.Time{}, outcomes: map[string]Result{}}
}

func TestMixedGroupUsesOneReservationAndIndependentReceipts(t *testing.T) {
	dir := stateDir(t)
	members := mixedPending(t, dir)
	s := batchServer(dir)
	reserved, prepared, announced := 0, 0, 0
	r := &reservationFixture{}
	s.Reserve = func(context.Context, Message) (Reservation, error) {
		reserved++
		if unread, _ := PeekUnread(dir, "api", s.Epoch); len(unread) != 0 {
			t.Fatal("group readable before reservation")
		}
		return r, nil
	}
	r.prepare = func(fn func(string) error) error {
		prepared++
		if r.closed {
			t.Fatal("member dropped shared fence")
		}
		return fn("root-A")
	}
	r.deliver = func(_ context.Context, m Message) Result {
		announced++
		if r.closed || len(m.Batch) != 4 || m.Unread != 4 {
			t.Fatal("group lost reservation or membership")
		}
		available, err := AvailableUnread(dir, "api", s.Epoch)
		if err != nil || len(available) != 4 {
			t.Fatal("native signal preceded member readability")
		}
		for i, member := range m.Batch {
			if member.ID != members[i].ID || member.Kind != members[i].Kind || member.Text != members[i].Text {
				t.Fatal("merged message identities or bodies")
			}
			if Owed(member) && must(deliveryThread(dir, "api", member.ID)) != "root-A" {
				t.Fatal("missing per-member target association")
			}
		}
		// A fast selected read remains final even if the shared ACK is lost.
		if err := state.WithMailboxLock(context.Background(), dir, "api", func() error { return MarkRead(dir, "api", s.Epoch, m.Batch[0], true) }); err != nil {
			t.Fatal(err)
		}
		return Result{State: Failed, Detail: "ACK lost"}
	}
	s.drain(context.Background())
	s.drain(context.Background())
	if reserved != 1 || prepared != 4 || announced != 1 || !r.closed {
		t.Fatalf("reserve=%d prepare=%d announce=%d closed=%v", reserved, prepared, announced, r.closed)
	}
	for i, m := range members {
		status, ok, _ := ReadStatus(dir, "api", m.ID)
		want := Failed
		if i == 0 {
			want = Read
		}
		if !ok || status.State != want || status.ReportAvailable != (m.Kind == Finished) {
			t.Fatalf("member outcome %d: %+v", i, status)
		}
	}
	unread, _ := AvailableUnread(dir, "api", s.Epoch)
	if len(unread) != 1 || unread[0].Kind != Finished {
		t.Fatal("failed group lost report or retained unaccepted task")
	}
}

func TestGroupCollectsArrivalsWhileReservationWaits(t *testing.T) {
	dir := stateDir(t)
	s := batchServer(dir)
	first := message("first")
	first.ToEpoch = s.Epoch
	if err := Put(dir, first); err != nil {
		t.Fatal(err)
	}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var captured Message
	reservations := 0
	r := &reservationFixture{prepare: func(fn func(string) error) error { return fn("root-A") }, deliver: func(_ context.Context, m Message) Result { captured = m; return Result{State: Delivered} }}
	s.Reserve = func(context.Context, Message) (Reservation, error) {
		reservations++
		close(entered)
		<-release
		return r, nil
	}
	go func() { defer close(done); s.drain(context.Background()) }()
	<-entered
	later := mixedPending(t, dir)
	close(release)
	<-done
	if reservations != 1 || len(captured.Batch) != 5 {
		t.Fatalf("readiness wait split pending group: reservations=%d members=%d", reservations, len(captured.Batch))
	}
	for _, m := range append([]Message{first}, later...) {
		if status, ok, _ := ReadStatus(dir, "api", m.ID); !ok || status.State != Delivered {
			t.Fatalf("missing group ACK for %s", m.ID)
		}
	}
}

func TestPendingRetryDoesNotDelayFreshMailOrReplayAcceptedMembers(t *testing.T) {
	dir := stateDir(t)
	s := batchServer(dir)
	members := mixedPending(t, dir)
	var groups []Message
	s.Deliver = func(_ context.Context, m Message) Result {
		groups = append(groups, m)
		if len(groups) == 1 {
			return Result{State: Pending}
		}
		return Result{State: Delivered}
	}
	s.drain(context.Background())
	later := message("during pending")
	later.ToEpoch = s.Epoch
	if err := Put(dir, later); err != nil {
		t.Fatal(err)
	}
	s.drain(context.Background())
	if len(groups) != 2 || groups[1].ID != later.ID || groups[1].Unread != 1 {
		t.Fatal("pending retry held fresh eligible mail")
	}
	for id := range s.attempts {
		s.attempts[id] = time.Now().Add(-retryInterval)
	}
	s.drain(context.Background())
	s.drain(context.Background())
	if len(groups) != 3 || len(groups[2].Batch) != len(members) {
		t.Fatal("retry lost pending group or replayed accepted member")
	}
	for i, m := range groups[2].Batch {
		if m.ID != members[i].ID {
			t.Fatal("retry membership changed")
		}
	}
	next := message("after acceptance")
	next.ToEpoch = s.Epoch
	if err := Put(dir, next); err != nil {
		t.Fatal(err)
	}
	s.drain(context.Background())
	s.drain(context.Background())
	if len(groups) != 4 || groups[3].ID != next.ID || groups[3].Unread != 1 {
		t.Fatal("new mail waited for overview or replayed old unread")
	}
}

func TestGroupRefusalKeepsReportsAndDoesNotAcquireAgain(t *testing.T) {
	dir := stateDir(t)
	s := batchServer(dir)
	members := mixedPending(t, dir)
	calls := 0
	s.Reserve = func(context.Context, Message) (Reservation, error) { calls++; return nil, errors.New("not ready") }
	s.Deliver = func(context.Context, Message) Result { t.Fatal("sent despite reservation refusal"); return Result{} }
	s.drain(context.Background())
	s.drain(context.Background())
	if calls != 1 {
		t.Fatal("conflicting/repeated reservations")
	}
	for _, m := range members {
		status, ok, _ := ReadStatus(dir, "api", m.ID)
		if !ok || status.State != Failed || status.ReportAvailable != IsReport(m) {
			t.Fatalf("refusal lost identity: %+v", status)
		}
	}
	unread, _ := AvailableUnread(dir, "api", s.Epoch)
	if len(unread) != 1 || unread[0].Kind != Finished {
		t.Fatal("refusal lost report")
	}
}
