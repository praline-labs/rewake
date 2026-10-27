package delivery

import (
	"context"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
)

func TestFailedReportNoticeRemainsReadable(t *testing.T) {
	for _, kind := range []mail.Kind{mail.Finished, mail.Error, mail.Stopped} {
		t.Run(string(kind), func(t *testing.T) {
			dir := stateDir(t)
			m := message("completed result")
			m.Kind = kind
			m.ToEpoch = "1.1"
			m.FromEpoch = "2.2"
			if err := mail.Put(dir, m); err != nil {
				t.Fatal(err)
			}
			calls := 0
			s := &Server{Dir: dir, Name: m.To, Epoch: m.ToEpoch, attempts: map[string]time.Time{}, outcomes: map[string]mail.Result{}, Deliver: func(context.Context, mail.Message) mail.Result {
				calls++
				return mail.Result{State: mail.Failed, Detail: "cannot identify the TUI among 2 loaded root threads"}
			}}
			s.drain(context.Background())
			status, ok, _ := mail.ReadStatus(dir, m.To, m.ID)
			if !ok || status.State != mail.Failed {
				t.Fatalf("notification failure lost: %+v", status)
			}
			available, err := mail.Open(dir, m.To, m.ToEpoch).AvailableUnread()
			if err != nil || len(available) != 1 {
				t.Fatalf("failed notice erased completed report: unread=%v err=%v", available, err)
			}
			s.drain(context.Background())
			if calls != 1 {
				t.Fatalf("notification retried %d times", calls)
			}
			if err := mail.Open(dir, m.To, m.ToEpoch).MarkRead(m, true); err != nil {
				t.Fatal(err)
			}
			if waits := mail.Open(dir, m.To, m.ToEpoch).Waiters(); len(waits) != 0 {
				t.Fatalf("report creates self-report loop: %+v", waits)
			}
		})
	}
}
