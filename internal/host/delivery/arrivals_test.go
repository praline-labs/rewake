package delivery

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
)

func TestSeparatedArrivalsDispatchWithoutOverview(t *testing.T) {
	dir := stateDir(t)
	var wakes atomic.Int32
	ready, done := make(chan struct{}), make(chan struct{})
	server := &Server{Dir: dir, Name: "api", Epoch: "owner-example", Ready: func() { close(ready) }, Deliver: func(context.Context, mail.Message) mail.Result {
		wakes.Add(1)
		return mail.Result{State: mail.Delivered}
	}}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { defer close(done); server.Serve(ctx) }()
	defer func() { cancel(); <-done }()
	<-ready
	// Each letter is sent once the one before it was announced, so no two
	// share a collection window, whatever the machine's speed.
	for i := range 4 {
		m := message("availability/departure fixture")
		m.Kind, m.ToEpoch = mail.Note, "owner-example"
		if err := mail.Put(dir, m); err != nil {
			t.Fatal(err)
		}
		t.Logf("queued message %d without recipient overview", i+1)
		for end := time.Now().Add(5 * time.Second); int(wakes.Load()) <= i; time.Sleep(5 * time.Millisecond) {
			if time.Now().After(end) {
				t.Fatalf("message %d was not announced", i+1)
			}
		}
	}
	// Long enough for a replayed announcement to show as a fifth wake.
	time.Sleep(2 * collectionInterval)
	cancel()
	<-done
	unread, err := mail.Open(dir, "api", "owner-example").PeekUnread()
	if err != nil || len(unread) != 4 {
		t.Fatalf("unread=%d err=%v", len(unread), err)
	}
	if got := wakes.Load(); got != 4 {
		t.Fatalf("separated arrivals before recipient overview produced %d visible wakes; want 4", got)
	}
}
