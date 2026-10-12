package delivery

import (
	"context"
	"testing"

	"github.com/praline-labs/rewake/internal/core/mail"
)

func TestSocketNeverSuppressesLaterMail(t *testing.T) {
	dir := stateDir(t)
	s := batchServer(dir)
	calls := 0
	s.Deliver = func(_ context.Context, m mail.Message) mail.Result {
		calls++
		if m.Unread != 1 || len(m.Batch) != 0 {
			t.Fatal("old unread inflated new fixed announcement")
		}
		return mail.Result{State: mail.Delivered, Via: "socket"}
	}
	for _, kind := range []mail.Kind{mail.Task, mail.Note, mail.Error, mail.Stopped} {
		m := message("snapshot")
		m.Kind, m.ToEpoch = kind, s.Epoch
		if err := mail.Put(dir, m); err != nil {
			t.Fatal(err)
		}
		s.drain(context.Background())
		st, ok, _ := mail.ReadStatus(dir, s.Name, m.ID)
		if !ok || st.State != mail.Delivered || st.Detail != "" {
			t.Fatalf("unexpected delivery result: %+v", st)
		}
	}
	if calls != 4 {
		t.Fatal("uncorrelated socket transport suppressed new mail")
	}
	s.drain(context.Background())
	if calls != 4 {
		t.Fatal("old unread replay")
	}
	unread, _ := mail.PeekUnread(dir, s.Name, s.Epoch)
	if len(unread) != 4 {
		t.Fatal("fallback lost independent messages")
	}
}
