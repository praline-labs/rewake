package inbox

import (
	"context"
	"testing"
	"time"
)

func TestFailedReportNoticeRemainsReadable(t *testing.T) {
	for _, kind := range []Kind{Finished, Error, Stopped} {
		t.Run(string(kind), func(t *testing.T) {
			dir := stateDir(t)
			m := message("completed result")
			m.Kind = kind
			m.ToEpoch = "1.1"
			m.FromEpoch = "2.2"
			if err := Put(dir, m); err != nil {
				t.Fatal(err)
			}
			calls := 0
			s := &Server{Dir: dir, Name: m.To, Epoch: m.ToEpoch, attempts: map[string]time.Time{}, outcomes: map[string]Result{}, Deliver: func(context.Context, Message) Result {
				calls++
				return Result{State: Failed, Detail: "cannot identify the TUI among 2 loaded root threads"}
			}}
			s.drain(context.Background())
			status, ok, _ := ReadStatus(dir, m.To, m.ID)
			if !ok || status.State != Failed {
				t.Fatalf("notification failure lost: %+v", status)
			}
			available, err := AvailableUnread(dir, m.To, m.ToEpoch)
			if err != nil || len(available) != 1 {
				t.Fatalf("failed notice erased completed report: unread=%v err=%v", available, err)
			}
			s.drain(context.Background())
			if calls != 1 {
				t.Fatalf("notification retried %d times", calls)
			}
			if err := MarkRead(dir, m.To, m.ToEpoch, m, true); err != nil {
				t.Fatal(err)
			}
			if waits := Waiters(dir, m.To, m.ToEpoch); len(waits) != 0 {
				t.Fatalf("report creates self-report loop: %+v", waits)
			}
		})
	}
}
