package mail

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// A mailbox that is removed and recreated leaves the watch pointing at nothing.
// Without asking for a new one, delivery quietly falls back to the poll forever.
func TestWatchSurvivesAReplacedMailbox(t *testing.T) {
	dir := stateDir(t)
	mailbox := state.InboxPath(dir, "api")
	if err := state.EnsureSubdir(mailbox); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changed := watchMailbox(ctx, dir, "api")
	if changed == nil {
		t.Skip("no watch on this system")
	}

	// Replace the directory the watch was set on, with a gap in between: for a
	// while there is nothing to watch at all.
	if err := os.RemoveAll(mailbox); err != nil {
		t.Fatalf("remove: %v", err)
	}
	time.Sleep(600 * time.Millisecond)
	if err := state.EnsureSubdir(mailbox); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Drain whatever the removal itself produced. A closed channel is not an
	// event: it means the watch is over.
	deadline := time.After(time.Second)
drain:
	for {
		select {
		case _, open := <-changed:
			if !open {
				t.Fatal("the watch ended when the mailbox was replaced")
			}
		case <-deadline:
			break drain
		}
	}

	if err := Put(dir, message("after the mailbox was replaced")); err != nil {
		t.Fatalf("put: %v", err)
	}
	select {
	case _, open := <-changed:
		if !open {
			t.Fatal("the watch ended instead of reporting the new message")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the watch never came back after the mailbox was replaced")
	}
}

// A watch that has ended must not keep the server busy: a closed channel is
// always ready, and reading it in a loop spun a core.
func TestAnEndedWatchDoesNotSpin(t *testing.T) {
	previous := watch
	watch = func(context.Context, string, string) <-chan struct{} {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	t.Cleanup(func() { watch = previous })

	server := &Server{Dir: stateDir(t), Name: "api", Deliver: func(context.Context, Message) Result {
		return Result{State: Delivered}
	}}
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		server.Serve(ctx)
	}()

	before := cpuTime(t)
	time.Sleep(time.Second)
	used := cpuTime(t) - before
	cancel()
	<-finished

	if used > 300*time.Millisecond {
		t.Errorf("the server used %v of CPU in one idle second", used)
	}
}

func cpuTime(t *testing.T) time.Duration {
	t.Helper()
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		t.Fatalf("getrusage: %v", err)
	}
	return time.Duration(usage.Utime.Nano() + usage.Stime.Nano())
}
