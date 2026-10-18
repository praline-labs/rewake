//go:build rewaketest

package delivery

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// stuckWaiting makes the last step of settling a delivered message fail —
// the removal of its waiting copy in dir's mailbox of name — and counts the
// attempts so a test can tell the step was really reached.
func stuckWaiting(t *testing.T, dir, name, id string) (attempts func() int64) {
	t.Helper()
	attempts, restore := mail.FailWaitingRemoval(filepath.Join(state.InboxPath(dir, name), id+".json"))
	t.Cleanup(restore)
	return attempts
}

// A delivered message whose settling fails must not be turned into a failure
// when the session ends: its sender would say the whole thing again.
func TestShutdownKeepsADeliveredStatus(t *testing.T) {
	dir := stateDir(t)
	sent := message("the migration is merged")
	if err := mail.Put(dir, sent); err != nil {
		t.Fatalf("put: %v", err)
	}
	attempts := stuckWaiting(t, dir, "api", sent.ID)

	server := &Server{Dir: dir, Name: "api", Deliver: func(context.Context, mail.Message) mail.Result {
		return mail.Result{State: mail.Delivered, Via: "socket"}
	}}

	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		server.Serve(ctx)
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if status, ok, _ := mail.ReadStatus(dir, "api", sent.ID); ok && status.State == mail.Delivered {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			<-finished
			t.Fatal("the message was never delivered")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-finished

	status, ok, _ := mail.ReadStatus(dir, "api", sent.ID)
	if !ok || status.State != mail.Delivered {
		t.Fatalf("status after shutdown = %+v, want it to stay delivered", status)
	}
	if attempts() == 0 {
		t.Fatal("settling was never attempted, so the test proved nothing")
	}
	if _, err := os.Stat(filepath.Join(state.UnreadPath(dir, "api"), sent.ID+".json")); err != nil {
		t.Errorf("a delivered message is no longer readable after shutdown: %v", err)
	}
}

// The server has to survive its own bookkeeping being right: a message it has
// already settled is not delivered again on the next pass.
func TestSettledMessageIsNotDeliveredTwice(t *testing.T) {
	dir := stateDir(t)
	sent := message("hello")
	if err := mail.Put(dir, sent); err != nil {
		t.Fatalf("put: %v", err)
	}
	// The waiting copy cannot be removed, so the next pass sees it again.
	attempts := stuckWaiting(t, dir, "api", sent.ID)

	deliveries := 0
	server := &Server{Dir: dir, Name: "api", Deliver: func(context.Context, mail.Message) mail.Result {
		deliveries++
		return mail.Result{State: mail.Delivered}
	}}

	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		server.Serve(ctx)
	}()
	time.Sleep(3 * time.Second)
	cancel()
	<-finished

	if deliveries != 1 {
		t.Fatalf("the message was delivered %d times, want once", deliveries)
	}
	if attempts() == 0 {
		t.Fatal("settling was never attempted, so the test proved nothing")
	}
	if waiting, _ := mail.List(dir, "api"); len(waiting) != 1 {
		t.Fatalf("mailbox holds %v; the message left it, so no later pass could see it again", waiting)
	}
}
