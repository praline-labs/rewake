package delivery

import (
	"context"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/mail/mailtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

func mixedPending(t *testing.T, dir string) []mail.Message {
	t.Helper()
	var result []mail.Message
	for _, kind := range []mail.Kind{mail.Task, mail.Question, mail.Note, mail.Finished} {
		m := message(string(kind) + " preview\nfull body")
		m.Kind, m.FromEpoch, m.ToEpoch = kind, "sender-epoch", "receiver-epoch"
		if err := mail.Put(dir, m); err != nil {
			t.Fatal(err)
		}
		result = append(result, m)
	}
	return result
}

func batchServer(dir string) *Server {
	return &Server{Dir: dir, Name: "api", Epoch: "receiver-epoch", attempts: map[string]time.Time{}, outcomes: map[string]mail.Result{}}
}

func TestMixedGroupIsOneNoticeWithIndependentReceipts(t *testing.T) {
	dir := stateDir(t)
	members := mixedPending(t, dir)
	s := batchServer(dir)
	threads, announced := 0, 0
	s.Thread = func() (string, error) {
		threads++
		return "root-A", nil
	}
	s.Deliver = func(_ context.Context, m mail.Message) mail.Result {
		announced++
		if len(m.Batch) != 4 || m.Unread != 4 {
			t.Fatal("group lost membership")
		}
		available, err := mail.Open(dir, "api", s.Epoch).AvailableUnread()
		if err != nil || len(available) != 4 {
			t.Fatal("native signal preceded member readability")
		}
		for i, member := range m.Batch {
			if member.ID != members[i].ID || member.Kind != members[i].Kind || member.Text != members[i].Text {
				t.Fatal("merged message identities or bodies")
			}
			if mail.Owed(member) && must(mailtest.DeliveryThread(dir, "api", member.ID)) != "root-A" {
				t.Fatal("missing per-member target association")
			}
		}
		// A fast selected read remains final even if the shared ACK is lost.
		if err := state.WithMailboxLock(context.Background(), dir, "api", func() error { return mail.Open(dir, "api", s.Epoch).MarkRead(m.Batch[0], true) }); err != nil {
			t.Fatal(err)
		}
		return mail.Result{State: mail.Failed, Detail: "ACK lost"}
	}
	s.drain(context.Background())
	s.drain(context.Background())
	if threads != 2 || announced != 1 {
		t.Fatalf("threads=%d announce=%d", threads, announced)
	}
	for i, m := range members {
		status, ok, _ := mail.ReadStatus(dir, "api", m.ID)
		want := mail.Failed
		if i == 0 {
			want = mail.Read
		}
		if !ok || status.State != want || status.ReportAvailable != (m.Kind == mail.Finished) {
			t.Fatalf("member outcome %d: %+v", i, status)
		}
	}
	unread, _ := mail.Open(dir, "api", s.Epoch).AvailableUnread()
	if len(unread) != 1 || unread[0].Kind != mail.Finished {
		t.Fatal("failed group lost report or retained unaccepted task")
	}
}

func TestPendingRetryDoesNotDelayFreshMailOrReplayAcceptedMembers(t *testing.T) {
	dir := stateDir(t)
	s := batchServer(dir)
	members := mixedPending(t, dir)
	var groups []mail.Message
	s.Deliver = func(_ context.Context, m mail.Message) mail.Result {
		groups = append(groups, m)
		if len(groups) == 1 {
			return mail.Result{State: mail.Pending}
		}
		return mail.Result{State: mail.Delivered}
	}
	s.drain(context.Background())
	later := message("during pending")
	later.ToEpoch = s.Epoch
	if err := mail.Put(dir, later); err != nil {
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
	if err := mail.Put(dir, next); err != nil {
		t.Fatal(err)
	}
	s.drain(context.Background())
	s.drain(context.Background())
	if len(groups) != 4 || groups[3].ID != next.ID || groups[3].Unread != 1 {
		t.Fatal("new mail waited for overview or replayed old unread")
	}
}
