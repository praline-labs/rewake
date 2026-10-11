package mail

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail/mailtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// Messages from one pending pass produce one notice with both members.
func TestNoticeCountsTheWaitingMail(t *testing.T) {
	dir := stateDir(t)
	first, second := message("one"), message("two")
	for _, sent := range []Message{first, second} {
		if err := Put(dir, sent); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}

	var mu sync.Mutex
	counts := map[string]int{}
	server := &Server{Dir: dir, Name: "api", Deliver: func(_ context.Context, m Message) Result {
		mu.Lock()
		defer mu.Unlock()
		counts[m.ID] = m.Unread
		return Result{State: Delivered, Via: "socket"}
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
	server := &Server{Dir: dir, Name: "api", Epoch: "6.6", Deliver: func(context.Context, Message) Result {
		return Result{State: Delivered}
	}}
	serveUntil(t, server, func() bool { return len(Waiters(dir, "api", "5.5")) == 0 })
}

// The agent is told a message is waiting and runs rewake inbox straight away.
// By then the message has to be there.
func TestNoticeGoesOutOnceTheMessageIsReadable(t *testing.T) {
	dir := stateDir(t)
	sent := message("pull and rerun the smoke")
	if err := Put(dir, sent); err != nil {
		t.Fatalf("Put: %v", err)
	}

	readable := make(chan bool, 1)
	server := &Server{Dir: dir, Name: "api", Deliver: func(_ context.Context, m Message) Result {
		_, err := os.Stat(filepath.Join(state.UnreadPath(dir, "api"), m.ID+".json"))
		select {
		case readable <- err == nil:
		default:
		}
		return Result{State: Delivered, Via: "socket"}
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
	if err := Put(dir, sent); err != nil {
		t.Fatalf("Put: %v", err)
	}
	reads, notices := 0, 0
	server := &Server{Dir: dir, Name: "api", Epoch: "5.5", attempts: map[string]time.Time{}, outcomes: map[string]Result{}}
	server.Deliver = func(context.Context, Message) Result {
		notices++
		reads += len(readAll(t, dir, "5.5"))
		return Result{State: Pending, Detail: "queue not ready"}
	}

	server.drain(context.Background())
	server.attempts[sent.ID] = time.Now().Add(-2 * retryInterval)
	server.drain(context.Background())

	if reads != 1 || notices != 1 {
		t.Errorf("reads = %d, notices = %d; want the task read once and announced once", reads, notices)
	}
	if status, _, _ := ReadStatus(dir, "api", sent.ID); status.State != Read {
		t.Errorf("status = %s, want read", status.State)
	}
}

// Whatever the harness says after the agent has read the message — even that
// the notice failed — the message was read, and its sender must hear that.
func TestAReadIsNotUndoneByTheNoticeResult(t *testing.T) {
	for _, outcome := range []State{Failed, Delivered} {
		t.Run(string(outcome), func(t *testing.T) {
			dir := stateDir(t)
			sent := message("already acted on")
			if err := Put(dir, sent); err != nil {
				t.Fatalf("Put: %v", err)
			}
			server := &Server{Dir: dir, Name: "api", attempts: map[string]time.Time{}, outcomes: map[string]Result{}}
			server.Deliver = func(context.Context, Message) Result {
				readAll(t, dir, "")
				return Result{State: outcome, Detail: "said after the read"}
			}
			server.drain(context.Background())
			if status, _, _ := ReadStatus(dir, "api", sent.ID); status.State != Read {
				t.Errorf("status = %s, want read", status.State)
			}
		})
	}
}
