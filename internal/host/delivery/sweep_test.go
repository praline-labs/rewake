package delivery

import (
	"context"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
)

// A message arriving while the session runs is delivered at once, not when the
// next poll happens to come round. The ticker is deliberately slow, so a server
// that only polled would fail this.
func TestANewMessageIsNoticedWithoutWaitingForThePoll(t *testing.T) {
	dir := stateDir(t)

	delivered := make(chan string, 1)
	server := &Server{Dir: dir, Name: "api", Deliver: func(_ context.Context, m mail.Message) mail.Result {
		select {
		case delivered <- m.Text:
		default:
		}
		return mail.Result{State: mail.Delivered}
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
	if err := mail.Put(dir, message("pull and rerun the smoke")); err != nil {
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
