package mail

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// serveUntil runs a server until the condition holds or the deadline passes, so
// a test waits for the thing it is about rather than for a fixed pause.
func serveUntil(t *testing.T, server *Server, done func() bool) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		server.Serve(ctx)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			cancel()
			<-finished
			t.Fatal("the server did not reach the expected state in time")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-finished
}

func TestDeliveredMessageWaitsToBeRead(t *testing.T) {
	dir := stateDir(t)
	sent := message("pull and rerun the smoke")
	if err := Put(dir, sent); err != nil {
		t.Fatalf("put: %v", err)
	}

	var seen string
	server := &Server{Dir: dir, Name: "api", Deliver: func(_ context.Context, m Message) Result {
		seen = m.Text
		return Result{State: Delivered, Via: "socket"}
	}}
	// The status is captured while the server runs: stopping it refuses whatever
	// is still waiting, which would rewrite the status this test is about.
	var status Status
	serveUntil(t, server, func() bool {
		captured, ok, _ := ReadStatus(dir, "api", sent.ID)
		if ok && captured.State == Delivered {
			status = captured
			return true
		}
		return false
	})

	if status.Via != "socket" {
		t.Errorf("status = %+v, want the delivery path recorded", status)
	}
	if seen != sent.Text {
		t.Errorf("delivered text = %q, want %q", seen, sent.Text)
	}
	if _, err := os.Stat(filepath.Join(state.InboxPath(dir, "api"), sent.ID+".json")); !os.IsNotExist(err) {
		t.Error("a delivered message is still waiting in the mailbox")
	}
	if _, err := os.Stat(filepath.Join(state.UnreadPath(dir, "api"), sent.ID+".json")); err != nil {
		t.Errorf("a delivered message is not waiting to be read: %v", err)
	}
}

// A pending result means the receiver cannot take the message yet. It must stay
// in the mailbox, and its reason must be readable straight away: a sender
// waiting on a status should learn why now, not when the message finally lands.
func TestPendingMessageStaysAndIsRetried(t *testing.T) {
	dir := stateDir(t)
	sent := message("are you there")
	if err := Put(dir, sent); err != nil {
		t.Fatalf("put: %v", err)
	}

	attempts := 0
	server := &Server{Dir: dir, Name: "api", Deliver: func(context.Context, Message) Result {
		attempts++
		if attempts < 2 {
			return Result{State: Pending, Detail: "the session is not listening yet"}
		}
		return Result{State: Delivered, Via: "socket"}
	}}

	began := time.Now()
	serveUntil(t, server, func() bool {
		status, ok, _ := ReadStatus(dir, "api", sent.ID)
		return ok && status.State == Delivered
	})

	if attempts < 2 {
		t.Errorf("attempts = %d, want the message retried", attempts)
	}
	// Pending means the receiver cannot take it yet, so retrying four times a
	// second buys nothing and costs a connection attempt each time.
	if waited := time.Since(began); waited < retryInterval {
		t.Errorf("the retry came after %v, want it to wait at least %v", waited, retryInterval)
	}
}

func TestPendingReasonIsReadableBeforeDelivery(t *testing.T) {
	dir := stateDir(t)
	sent := message("hello")
	if err := Put(dir, sent); err != nil {
		t.Fatalf("put: %v", err)
	}

	server := &Server{Dir: dir, Name: "api", Deliver: func(context.Context, Message) Result {
		return Result{State: Pending, Detail: "the session has no conversation yet"}
	}}
	// The status is captured while the server still runs: stopping it refuses
	// whatever is waiting, which would overwrite the reason under test.
	var status Status
	serveUntil(t, server, func() bool {
		captured, ok, _ := ReadStatus(dir, "api", sent.ID)
		if ok && captured.Detail != "" {
			status = captured
			return true
		}
		return false
	})

	if status.State != Pending {
		t.Errorf("state = %q, want pending", status.State)
	}
	if status.Detail != "the session has no conversation yet" {
		t.Errorf("detail = %q, want the delivery reason", status.Detail)
	}
}

func TestExpiredMessageFails(t *testing.T) {
	dir := stateDir(t)
	old := message("stale")
	// Older than the TTL under test, younger than the default: a server that
	// ignored its own TTL would still call this one fresh.
	old.CreatedAt = time.Now().Add(-5 * time.Second)
	if err := Put(dir, old); err != nil {
		t.Fatalf("put: %v", err)
	}

	delivered := false
	server := &Server{Dir: dir, Name: "api", TTL: time.Second, Deliver: func(context.Context, Message) Result {
		delivered = true
		return Result{State: Delivered}
	}}
	serveUntil(t, server, func() bool {
		status, ok, _ := ReadStatus(dir, "api", old.ID)
		return ok && status.State == Failed
	})

	if delivered {
		t.Error("an expired message was still handed to the harness")
	}
	status, _, _ := ReadStatus(dir, "api", old.ID)
	if status.Detail == "" {
		t.Error("the refusal does not say why")
	}
}

// When the session ends, whatever is still waiting will never be delivered. A
// sender blocked on a status deserves to hear that instead of timing out.
func TestWaitingMessagesFailWhenTheSessionEnds(t *testing.T) {
	dir := stateDir(t)
	sent := message("still here?")
	if err := Put(dir, sent); err != nil {
		t.Fatalf("put: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	server := &Server{Dir: dir, Name: "api", Deliver: func(context.Context, Message) Result {
		return Result{State: Pending, Detail: "not listening yet"}
	}}
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		server.Serve(ctx)
	}()

	time.Sleep(400 * time.Millisecond)
	cancel()
	<-finished

	status, ok, _ := ReadStatus(dir, "api", sent.ID)
	if !ok || status.State != Failed {
		t.Fatalf("status = %+v, want failed", status)
	}
	// It also has to leave the mailbox: a refused message left waiting is
	// delivered by the next session that takes this name.
	waiting, err := List(dir, "api")
	if err != nil || len(waiting) != 0 {
		t.Fatalf("mailbox holds %v (%v), want it cleared", waiting, err)
	}
}
