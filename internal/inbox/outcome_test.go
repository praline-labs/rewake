package inbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Delivery is the step that cannot be undone. If the status write fails, the
// outcome still has to survive, or the next pass hands the harness the same
// message again — a message whose sender was already told it arrived.
func TestFailedStatusWriteDoesNotRedeliver(t *testing.T) {
	dir := stateDir(t)
	sent := message("pull and rerun the smoke")
	if err := Put(dir, sent); err != nil {
		t.Fatalf("put: %v", err)
	}
	// A directory where the status file belongs: writing it can only fail.
	if err := os.MkdirAll(statusPath(dir, "api", sent.ID), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	deliveries := 0
	server := &Server{Dir: dir, Name: "api", Deliver: func(context.Context, Message) Result {
		deliveries++
		return Result{State: Delivered, Via: "socket"}
	}}

	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		server.Serve(ctx)
	}()
	// Long enough for several passes and more than one retry interval.
	time.Sleep(3 * time.Second)
	cancel()
	<-finished

	if deliveries != 1 {
		t.Fatalf("the message was delivered %d times, want once", deliveries)
	}
}

// A delivered message whose archiving fails must not be turned into a failure
// when the session ends: its sender would say the whole thing again.
func TestShutdownKeepsADeliveredStatus(t *testing.T) {
	dir := stateDir(t)
	sent := message("the migration is merged")
	if err := Put(dir, sent); err != nil {
		t.Fatalf("put: %v", err)
	}
	// A file where the done/ directory belongs: archiving can only fail.
	if err := os.WriteFile(filepath.Join(dir, "inbox", "api", "done"), nil, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	server := &Server{Dir: dir, Name: "api", Deliver: func(context.Context, Message) Result {
		return Result{State: Delivered, Via: "socket"}
	}}

	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		server.Serve(ctx)
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if status, ok := ReadStatus(dir, "api", sent.ID); ok && status.State == Delivered {
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

	status, ok := ReadStatus(dir, "api", sent.ID)
	if !ok || status.State != Delivered {
		t.Fatalf("status after shutdown = %+v, want it to stay delivered", status)
	}
}

// A message written before epochs existed, or for a session that has ended, is
// not this session's mail even though the mailbox carries its name.
func TestMessageWithoutAnEpochIsRefused(t *testing.T) {
	dir := stateDir(t)
	old := message("for whoever is called api")
	old.ToEpoch = ""
	if err := Put(dir, old); err != nil {
		t.Fatalf("put: %v", err)
	}

	delivered := false
	server := &Server{Dir: dir, Name: "api", Epoch: "42.100", Deliver: func(context.Context, Message) Result {
		delivered = true
		return Result{State: Delivered}
	}}
	serveUntil(t, server, func() bool {
		status, ok := ReadStatus(dir, "api", old.ID)
		return ok && status.State == Failed
	})

	if delivered {
		t.Error("a message with no epoch was handed to the session")
	}
	status, _ := ReadStatus(dir, "api", old.ID)
	if status.Detail == "" {
		t.Error("the refusal does not say why")
	}
}

// The server has to survive its own bookkeeping being right: a message it has
// already settled is not delivered again on the next pass.
func TestSettledMessageIsNotDeliveredTwice(t *testing.T) {
	dir := stateDir(t)
	sent := message("hello")
	if err := Put(dir, sent); err != nil {
		t.Fatalf("put: %v", err)
	}
	// Archiving is blocked, so the message stays visible in the mailbox and the
	// next pass sees it again.
	if err := os.WriteFile(filepath.Join(dir, "inbox", "api", "done"), nil, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	deliveries := 0
	server := &Server{Dir: dir, Name: "api", Deliver: func(context.Context, Message) Result {
		deliveries++
		return Result{State: Delivered}
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
}

// Mail that arrives for another epoch while this session is running belongs to
// whoever takes the name next — a wrapper on its way out refusing it is how a
// live conversation was killed by a dead one.
func TestMailForTheNextOwnerIsLeftAlone(t *testing.T) {
	dir := stateDir(t)

	ours := message("for us")
	ours.ToEpoch = "42.100"
	if err := Put(dir, ours); err != nil {
		t.Fatalf("put: %v", err)
	}

	server := &Server{Dir: dir, Name: "api", Epoch: "42.100", Deliver: func(context.Context, Message) Result {
		return Result{State: Delivered, Via: "socket"}
	}}

	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		server.Serve(ctx)
	}()

	deadline := time.Now().Add(3 * time.Second)
	for {
		if status, ok := ReadStatus(dir, "api", ours.ID); ok && status.State == Delivered {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			<-finished
			t.Fatal("our own message was never delivered")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Written after this session started, for a session that is not it.
	theirs := message("for whoever takes the name next")
	theirs.ToEpoch = "43.200"
	if err := Put(dir, theirs); err != nil {
		t.Fatalf("put: %v", err)
	}
	time.Sleep(600 * time.Millisecond)

	if status, ok := ReadStatus(dir, "api", theirs.ID); ok {
		t.Fatalf("somebody else's message was answered while this session ran: %+v", status)
	}

	// Shutting down must not touch it either.
	cancel()
	<-finished

	if status, ok := ReadStatus(dir, "api", theirs.ID); ok {
		t.Fatalf("somebody else's message was refused on the way out: %+v", status)
	}
	waiting, err := list(dir, "api")
	if err != nil || len(waiting) != 1 || waiting[0].ID != theirs.ID {
		t.Fatalf("mailbox holds %v (%v), want the other session's message still waiting", waiting, err)
	}
}

// Mail addressed to a session that used this name before will never be
// delivered — this one took the name. It is refused once, at the start, rather
// than left to expire in silence.
func TestMailOfAPreviousSessionIsRefusedAtStartup(t *testing.T) {
	dir := stateDir(t)
	old := message("for the session that came before")
	old.ToEpoch = "41.50"
	if err := Put(dir, old); err != nil {
		t.Fatalf("put: %v", err)
	}

	server := &Server{Dir: dir, Name: "api", Epoch: "42.100", Deliver: func(context.Context, Message) Result {
		t.Error("somebody else's message was handed to the session")
		return Result{State: Delivered}
	}}
	serveUntil(t, server, func() bool {
		status, ok := ReadStatus(dir, "api", old.ID)
		return ok && status.State == Failed
	})
}

// Two messages written one after the other are delivered in that order, even
// when they land in the same millisecond: "do it" must not arrive before "here
// is what to do".
func TestOrderSurvivesTheSameMillisecond(t *testing.T) {
	dir := stateDir(t)

	var sent []Message
	for _, text := range []string{"first", "second", "third"} {
		one := message(text)
		if err := Put(dir, one); err != nil {
			t.Fatalf("put: %v", err)
		}
		sent = append(sent, one)
	}

	var order []string
	server := &Server{Dir: dir, Name: "api", Deliver: func(_ context.Context, m Message) Result {
		order = append(order, m.Text)
		return Result{State: Delivered}
	}}
	serveUntil(t, server, func() bool {
		status, ok := ReadStatus(dir, "api", sent[len(sent)-1].ID)
		return ok && status.State == Delivered
	})

	if len(order) != 3 || order[0] != "first" || order[1] != "second" || order[2] != "third" {
		t.Fatalf("delivered in the order %v, want them as they were written", order)
	}
}
