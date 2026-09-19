package inbox

import (
	"context"
	"testing"
)

func TestReviewSocketNeverSuppressesLaterMail(t *testing.T) {
	dir := stateDir(t)
	s := batchServer(dir)
	calls := 0
	s.Deliver = func(_ context.Context, m Message) Result {
		calls++
		if m.Unread != 1 || len(m.Batch) != 0 {
			t.Fatal("old unread inflated new fixed announcement")
		}
		return Result{State: Delivered, Via: "socket"}
	}
	for _, kind := range []Kind{Task, Note, Error, Stopped} {
		m := message("snapshot")
		m.Kind, m.ToEpoch = kind, s.Epoch
		if err := Put(dir, m); err != nil {
			t.Fatal(err)
		}
		s.drain(context.Background())
		st, ok := ReadStatus(dir, s.Name, m.ID)
		if !ok || st.State != Delivered || st.Detail != "" {
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
	unread, _ := PeekUnread(dir, s.Name, s.Epoch)
	if len(unread) != 4 {
		t.Fatal("fallback lost independent messages")
	}
}
