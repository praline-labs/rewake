package inbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

// A message arriving while the session runs is delivered at once, not when the
// next poll happens to come round. The ticker is deliberately slow, so a server
// that only polled would fail this.
func TestANewMessageIsNoticedWithoutWaitingForThePoll(t *testing.T) {
	dir := stateDir(t)

	delivered := make(chan string, 1)
	server := &Server{Dir: dir, Name: "api", Deliver: func(_ context.Context, m Message) Result {
		select {
		case delivered <- m.Text:
		default:
		}
		return Result{State: Delivered}
	}}

	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		server.Serve(ctx)
	}()
	defer func() {
		cancel()
		<-finished
	}()

	// Let the watch settle, then write a message the way a sender does.
	time.Sleep(200 * time.Millisecond)
	began := time.Now()
	if err := Put(dir, message("pull and rerun the smoke")); err != nil {
		t.Fatalf("put: %v", err)
	}

	select {
	case <-delivered:
		// Well inside the poll: a server that only polled would answer at the
		// tick, and the point of the watch is that it does not.
		if waited := time.Since(began); waited > pollInterval/2 {
			t.Errorf("the message waited %v, which is the poll rather than the watch", waited)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the message was never delivered")
	}
}

// Answered messages are kept for a while and then let go: a machine that runs
// for weeks should not collect a mailbox full of last month's conversations.
func TestFinishedMessagesAreSweptByAge(t *testing.T) {
	dir := stateDir(t)
	server := &Server{Dir: dir, Name: "api"}

	done := state.DonePath(dir, "api")
	if err := state.EnsureSubdir(done); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mailbox := state.InboxPath(dir, "api")

	old := time.Now().Add(-keepFinished - time.Hour)
	recent := time.Now().Add(-time.Minute)
	files := map[string]time.Time{
		filepath.Join(done, "old.json"):        old,
		filepath.Join(done, "recent.json"):     recent,
		filepath.Join(mailbox, "old.status"):   old,
		filepath.Join(mailbox, "fresh.status"): recent,
	}
	for path, when := range files {
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := os.Chtimes(path, when, when); err != nil {
			t.Fatalf("chtimes: %v", err)
		}
	}
	// A message still waiting is answered by the TTL, not by the sweep.
	waiting := message("still waiting")
	if err := Put(dir, waiting); err != nil {
		t.Fatalf("put: %v", err)
	}
	stale := filepath.Join(mailbox, waiting.ID+".json")
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	server.sweepFinished()

	for path, when := range files {
		_, err := os.Stat(path)
		if when == old && !os.IsNotExist(err) {
			t.Errorf("%s survived the sweep", filepath.Base(path))
		}
		if when == recent && err != nil {
			t.Errorf("%s was swept too early: %v", filepath.Base(path), err)
		}
	}
	if _, err := os.Stat(stale); err != nil {
		t.Errorf("a message that is still waiting was swept: %v", err)
	}
}
