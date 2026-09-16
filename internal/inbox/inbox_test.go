package inbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

func stateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Setenv(state.DirEnv, dir)
	resolved, err := state.Dir()
	if err != nil {
		t.Fatalf("state.Dir: %v", err)
	}
	return resolved
}

func message(text string) Message {
	return Message{ID: NewID(), From: "web", To: "api", Text: text, CreatedAt: time.Now()}
}

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

func TestDeliveredMessageIsArchivedWithItsStatus(t *testing.T) {
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
	serveUntil(t, server, func() bool {
		status, ok := ReadStatus(dir, "api", sent.ID)
		return ok && status.State == Delivered
	})

	if seen != sent.Text {
		t.Errorf("delivered text = %q, want %q", seen, sent.Text)
	}
	if _, err := os.Stat(filepath.Join(state.InboxPath(dir, "api"), sent.ID+".json")); !os.IsNotExist(err) {
		t.Error("a delivered message is still waiting in the mailbox")
	}
	if _, err := os.Stat(filepath.Join(state.DonePath(dir, "api"), sent.ID+".json")); err != nil {
		t.Errorf("a delivered message was not archived: %v", err)
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

	// The retry interval is two seconds, so this also proves the wait is real.
	serveUntil(t, server, func() bool {
		status, ok := ReadStatus(dir, "api", sent.ID)
		return ok && status.State == Delivered
	})

	if attempts < 2 {
		t.Errorf("attempts = %d, want the message retried", attempts)
	}
}

func TestPendingReasonIsReadableBeforeDelivery(t *testing.T) {
	dir := stateDir(t)
	sent := message("hello")
	if err := Put(dir, sent); err != nil {
		t.Fatalf("put: %v", err)
	}

	server := &Server{Dir: dir, Name: "api", Deliver: func(context.Context, Message) Result {
		return Result{State: Pending, Detail: "the codex session has no conversation yet"}
	}}
	// The status is captured while the server still runs: stopping it refuses
	// whatever is waiting, which would overwrite the reason under test.
	var status Status
	serveUntil(t, server, func() bool {
		captured, ok := ReadStatus(dir, "api", sent.ID)
		if ok && captured.Detail != "" {
			status = captured
			return true
		}
		return false
	})

	if status.State != Pending {
		t.Errorf("state = %q, want pending", status.State)
	}
	if status.Detail != "the codex session has no conversation yet" {
		t.Errorf("detail = %q, want the delivery reason", status.Detail)
	}
}

func TestExpiredMessageFails(t *testing.T) {
	dir := stateDir(t)
	old := message("stale")
	old.CreatedAt = time.Now().Add(-time.Hour)
	if err := Put(dir, old); err != nil {
		t.Fatalf("put: %v", err)
	}

	delivered := false
	server := &Server{Dir: dir, Name: "api", TTL: time.Minute, Deliver: func(context.Context, Message) Result {
		delivered = true
		return Result{State: Delivered}
	}}
	serveUntil(t, server, func() bool {
		status, ok := ReadStatus(dir, "api", old.ID)
		return ok && status.State == Failed
	})

	if delivered {
		t.Error("an expired message was still handed to the harness")
	}
	status, _ := ReadStatus(dir, "api", old.ID)
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

	status, ok := ReadStatus(dir, "api", sent.ID)
	if !ok || status.State != Failed {
		t.Fatalf("status = %+v, want failed", status)
	}
}

func TestAwaitReturnsTheFinalStatus(t *testing.T) {
	dir := stateDir(t)
	sent := message("hi")
	if err := Put(dir, sent); err != nil {
		t.Fatalf("put: %v", err)
	}

	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = writeStatus(dir, "api", sent.ID, Result{State: Delivered, Via: "socket"})
	}()

	status, ok := Await(dir, "api", sent.ID, 3*time.Second)
	if !ok || status.State != Delivered {
		t.Fatalf("Await = %+v, %v; want delivered", status, ok)
	}
}

func TestAwaitGivesUpWithoutAStatus(t *testing.T) {
	dir := stateDir(t)
	if _, ok := Await(dir, "api", "missing", 200*time.Millisecond); ok {
		t.Error("Await reported a status that was never written")
	}
}

func TestIDsSortByTime(t *testing.T) {
	first := NewID()
	time.Sleep(2 * time.Millisecond)
	second := NewID()
	if !(first < second) {
		t.Errorf("ids do not sort by time: %q then %q", first, second)
	}
}
