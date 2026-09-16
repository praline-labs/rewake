package inbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

// A reader stuck on its output holds the mailbox. The server must still end
// when its session does: waiting on the lock without end kept the wrapper alive
// after its harness had gone.
func TestAStuckReaderDoesNotHoldTheServer(t *testing.T) {
	dir := stateDir(t)
	held := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_ = state.WithMailboxLock(context.Background(), dir, "api", func() error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	defer close(release)

	sent := message("waits for the reader")
	if err := Put(dir, sent); err != nil {
		t.Fatalf("Put: %v", err)
	}
	server := &Server{Dir: dir, Name: "api", Deliver: func(context.Context, Message) Result {
		return Result{State: Delivered}
	}}
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		server.Serve(ctx)
	}()
	time.Sleep(1500 * time.Millisecond)
	cancel()

	select {
	case <-finished:
	case <-time.After(shutdownLockWait + 2*time.Second):
		t.Fatal("the server did not end while a reader held the mailbox")
	}
}

// A lock nobody can take does not stop delivery: the server is then the only
// writer, and the sender hears what happened instead of a silent pending.
func TestAnUnusableLockDoesNotStopDelivery(t *testing.T) {
	dir := stateDir(t)
	if err := os.MkdirAll(filepath.Join(state.InboxPath(dir, "api"), ".lock"), 0o700); err != nil {
		t.Fatalf("block: %v", err)
	}
	sent := message("still delivered")
	if err := Put(dir, sent); err != nil {
		t.Fatalf("Put: %v", err)
	}
	server := &Server{Dir: dir, Name: "api", Deliver: func(context.Context, Message) Result {
		return Result{State: Delivered, Via: "socket"}
	}}
	serveUntil(t, server, func() bool {
		status, ok := ReadStatus(dir, "api", sent.ID)
		return ok && status.State == Delivered
	})
}

// A reader running inside delivery — the agent reads the moment it is told —
// takes the real lock, and must get it: delivery does not hold the mailbox.
func TestAReaderDuringDeliveryGetsTheLock(t *testing.T) {
	dir := stateDir(t)
	sent := message("task")
	sent.ToEpoch = "5.5"
	if err := Put(dir, sent); err != nil {
		t.Fatalf("Put: %v", err)
	}
	server := &Server{Dir: dir, Name: "api", Epoch: "5.5", attempts: map[string]time.Time{}, outcomes: map[string]Result{}}
	server.Deliver = func(context.Context, Message) Result {
		readAll(t, dir, "5.5")
		return Result{State: Pending}
	}
	done := make(chan struct{})
	go func() {
		server.drain(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("a reader could not get the mailbox while a notice was on its way")
	}
	if status, _ := ReadStatus(dir, "api", sent.ID); status.State != Read {
		t.Errorf("status = %s, want read", status.State)
	}
}
