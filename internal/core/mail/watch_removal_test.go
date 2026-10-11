//go:build rewakefault

package mail

import (
	"context"
	"testing"
	"time"
)

// The server writes into the mailbox itself — statuses, temporary files — and
// reacting to its own writes turned one unarchivable message into a loop: write,
// event, pass, write again, hundreds of times a second.
func TestOwnWritesDoNotFeedTheWatch(t *testing.T) {
	dir := stateDir(t)
	sent := message("hello")
	if err := Put(dir, sent); err != nil {
		t.Fatalf("put: %v", err)
	}
	// Settling can only fail, so the message stays visible and every pass that
	// is allowed to writes its status again.
	attempts := stuckWaiting(t, dir, "api", sent.ID)

	server := &Server{Dir: dir, Name: "api", Deliver: func(context.Context, Message) Result {
		return Result{State: Delivered, Via: "socket"}
	}}

	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		server.Serve(ctx)
	}()
	// Passes run once a second; a retry is allowed every two. Four and a half
	// seconds hold one delivery and one retry, against four without the limit.
	time.Sleep(4500 * time.Millisecond)
	// Counted while the server still runs: a loop stops with the server.
	settles := attempts()
	cancel()
	<-finished

	if settles == 0 {
		t.Fatal("settling was never attempted, so the test proved nothing")
	}
	if settles > 2 {
		t.Fatalf("settling ran %d times in four and a half seconds: the retry limit is not holding", settles)
	}
}
