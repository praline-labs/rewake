package delivery

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/mail/mailtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// Messages from one pending pass produce one notice with both members.
func TestNoticeCountsTheWaitingMail(t *testing.T) {
	dir := stateDir(t)
	first, second := message("one"), message("two")
	for _, sent := range []mail.Message{first, second} {
		if err := mail.Put(dir, sent); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}

	var mu sync.Mutex
	counts := map[string]int{}
	server := &Server{Dir: dir, Name: "api", Deliver: func(_ context.Context, m mail.Message) mail.Result {
		mu.Lock()
		defer mu.Unlock()
		counts[m.ID] = m.Unread
		return mail.Result{State: mail.Delivered, Via: "socket"}
	}}
	serveUntil(t, server, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(counts) == 1
	})

	for _, count := range counts {
		if count != 2 {
			t.Errorf("counts = %v, want one grouped count of two", counts)
		}
	}
}

// A new run of the name forgets what the previous run was waited on for.
func TestANewRunForgetsTheOldWaits(t *testing.T) {
	dir := stateDir(t)
	if err := mailtest.Owe(dir, "api", "5.5", "web", "40.4", "m0"); err != nil {
		t.Fatalf("Owe: %v", err)
	}
	server := &Server{Dir: dir, Name: "api", Epoch: "6.6", Deliver: func(context.Context, mail.Message) mail.Result {
		return mail.Result{State: mail.Delivered}
	}}
	serveUntil(t, server, func() bool { return len(mail.Open(dir, "api", "5.5").Waiters()) == 0 })
}

// The agent is told a message is waiting and runs rewake inbox straight away.
// By then the message has to be there.
func TestNoticeGoesOutOnceTheMessageIsReadable(t *testing.T) {
	dir := stateDir(t)
	sent := message("pull and rerun the smoke")
	if err := mail.Put(dir, sent); err != nil {
		t.Fatalf("Put: %v", err)
	}

	readable := make(chan bool, 1)
	server := &Server{Dir: dir, Name: "api", Deliver: func(_ context.Context, m mail.Message) mail.Result {
		_, err := os.Stat(filepath.Join(state.UnreadPath(dir, "api"), m.ID+".json"))
		select {
		case readable <- err == nil:
		default:
		}
		return mail.Result{State: mail.Delivered, Via: "socket"}
	}}
	var seen bool
	serveUntil(t, server, func() bool {
		select {
		case seen = <-readable:
			return true
		default:
			return false
		}
	})
	if !seen {
		t.Error("the notice went out before the message could be read")
	}
}

// A message linked for an earlier attempt can be read before the next notice
// goes out. The read stands: the task is not announced, or handed out, again.
func TestAReadDuringAPendingNoticeStands(t *testing.T) {
	dir := stateDir(t)
	sent := message("execute once")
	sent.ToEpoch = "5.5"
	if err := mail.Put(dir, sent); err != nil {
		t.Fatalf("Put: %v", err)
	}
	reads, notices := 0, 0
	server := &Server{Dir: dir, Name: "api", Epoch: "5.5", attempts: map[string]time.Time{}, outcomes: map[string]mail.Result{}}
	server.Deliver = func(context.Context, mail.Message) mail.Result {
		notices++
		reads += len(readAll(t, dir, "5.5"))
		return mail.Result{State: mail.Pending, Detail: "queue not ready"}
	}

	server.drain(context.Background())
	server.attempts[sent.ID] = time.Now().Add(-2 * retryInterval)
	server.drain(context.Background())

	if reads != 1 || notices != 1 {
		t.Errorf("reads = %d, notices = %d; want the task read once and announced once", reads, notices)
	}
	if status, _, _ := mail.ReadStatus(dir, "api", sent.ID); status.State != mail.Read {
		t.Errorf("status = %s, want read", status.State)
	}
}

// Whatever the harness says after the agent has read the message — even that
// the notice failed — the message was read, and its sender must hear that.
func TestAReadIsNotUndoneByTheNoticeResult(t *testing.T) {
	for _, outcome := range []mail.State{mail.Failed, mail.Delivered} {
		t.Run(string(outcome), func(t *testing.T) {
			dir := stateDir(t)
			sent := message("already acted on")
			if err := mail.Put(dir, sent); err != nil {
				t.Fatalf("Put: %v", err)
			}
			server := &Server{Dir: dir, Name: "api", attempts: map[string]time.Time{}, outcomes: map[string]mail.Result{}}
			server.Deliver = func(context.Context, mail.Message) mail.Result {
				readAll(t, dir, "")
				return mail.Result{State: outcome, Detail: "said after the read"}
			}
			server.drain(context.Background())
			if status, _, _ := mail.ReadStatus(dir, "api", sent.ID); status.State != mail.Read {
				t.Errorf("status = %s, want read", status.State)
			}
		})
	}
}
