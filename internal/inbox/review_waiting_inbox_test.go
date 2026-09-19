package inbox

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestSeparatedArrivalsDispatchWithoutOverview(t *testing.T) {
	dir := stateDir(t)
	var wakes atomic.Int32
	ready, done := make(chan struct{}), make(chan struct{})
	server := &Server{Dir: dir, Name: "api", Epoch: "owner-example", Ready: func() { close(ready) }, Deliver: func(context.Context, Message) Result {
		wakes.Add(1)
		return Result{State: Delivered}
	}}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { defer close(done); server.Serve(ctx) }()
	defer func() { cancel(); <-done }()
	<-ready
	for i, delay := range []time.Duration{0, 2 * time.Second, 3 * time.Second, 2 * time.Second} {
		if delay > 0 {
			timer := time.NewTimer(delay)
			<-timer.C
		}
		m := message("availability/departure fixture")
		m.Kind, m.ToEpoch = Note, "owner-example"
		if err := Put(dir, m); err != nil {
			t.Fatal(err)
		}
		t.Logf("queued message %d without recipient overview", i+1)
	}
	timer := time.NewTimer(2 * collectionInterval)
	<-timer.C
	cancel()
	<-done
	unread, err := PeekUnread(dir, "api", "owner-example")
	if err != nil || len(unread) != 4 {
		t.Fatalf("unread=%d err=%v", len(unread), err)
	}
	if got := wakes.Load(); got != 4 {
		t.Fatalf("separated arrivals before recipient overview produced %d visible wakes; want 4", got)
	}
}
