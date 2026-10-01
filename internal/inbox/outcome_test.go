package inbox

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/state"
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
	// Writing the status can only fail.
	failStatusWrites(t, dir, "api", sent.ID)

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

// stuckWaiting makes the last step of settling a delivered message fail, and
// counts the attempts so a test can tell the step was really reached.
func stuckWaiting(t *testing.T) *atomic.Int64 {
	t.Helper()
	attempts := &atomic.Int64{}
	previous := removeWaiting
	removeWaiting = func(string) error {
		attempts.Add(1)
		return os.ErrPermission
	}
	t.Cleanup(func() { removeWaiting = previous })
	return attempts
}

// A delivered message whose settling fails must not be turned into a failure
// when the session ends: its sender would say the whole thing again.
func TestShutdownKeepsADeliveredStatus(t *testing.T) {
	dir := stateDir(t)
	sent := message("the migration is merged")
	if err := Put(dir, sent); err != nil {
		t.Fatalf("put: %v", err)
	}
	attempts := stuckWaiting(t)

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
		if status, ok, _ := ReadStatus(dir, "api", sent.ID); ok && status.State == Delivered {
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

	status, ok, _ := ReadStatus(dir, "api", sent.ID)
	if !ok || status.State != Delivered {
		t.Fatalf("status after shutdown = %+v, want it to stay delivered", status)
	}
	if attempts.Load() == 0 {
		t.Fatal("settling was never attempted, so the test proved nothing")
	}
	if _, err := os.Stat(filepath.Join(state.UnreadPath(dir, "api"), sent.ID+".json")); err != nil {
		t.Errorf("a delivered message is no longer readable after shutdown: %v", err)
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
		status, ok, _ := ReadStatus(dir, "api", old.ID)
		return ok && status.State == Failed
	})

	if delivered {
		t.Error("a message with no epoch was handed to the session")
	}
	status, _, _ := ReadStatus(dir, "api", old.ID)
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
	// The waiting copy cannot be removed, so the next pass sees it again.
	attempts := stuckWaiting(t)

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
	if attempts.Load() == 0 {
		t.Fatal("settling was never attempted, so the test proved nothing")
	}
	if waiting, _ := list(dir, "api"); len(waiting) != 1 {
		t.Fatalf("mailbox holds %v; the message left it, so no later pass could see it again", waiting)
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
		if status, ok, _ := ReadStatus(dir, "api", ours.ID); ok && status.State == Delivered {
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

	if status, ok, _ := ReadStatus(dir, "api", theirs.ID); ok {
		t.Fatalf("somebody else's message was answered while this session ran: %+v", status)
	}

	// Shutting down must not touch it either.
	cancel()
	<-finished

	if status, ok, _ := ReadStatus(dir, "api", theirs.ID); ok {
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
		status, ok, _ := ReadStatus(dir, "api", old.ID)
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
		for _, member := range m.Batch {
			order = append(order, member.Text)
		}
		return Result{State: Delivered}
	}}
	serveUntil(t, server, func() bool {
		status, ok, _ := ReadStatus(dir, "api", sent[len(sent)-1].ID)
		return ok && status.State == Delivered
	})

	if len(order) != 3 || order[0] != "first" || order[1] != "second" || order[2] != "third" {
		t.Fatalf("delivered in the order %v, want them as they were written", order)
	}
}

// A status write that failed a moment before the session ended is written on
// the way out, not skipped for being too soon after the last try: after exit
// the outcome exists nowhere else, and the next session with the name would
// refuse a message that was delivered.
func TestShutdownWritesAnOutcomeKeptOnlyInMemory(t *testing.T) {
	dir := stateDir(t)
	sent := message("the migration is merged")
	sent.ToEpoch = "5.5"
	if err := Put(dir, sent); err != nil {
		t.Fatalf("put: %v", err)
	}
	unblock := failStatusWrites(t, dir, "api", sent.ID)

	delivered := make(chan struct{})
	server := &Server{Dir: dir, Name: "api", Epoch: "5.5", Deliver: func(context.Context, Message) Result {
		close(delivered)
		return Result{State: Delivered, Via: "socket"}
	}}
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		server.Serve(ctx)
	}()
	<-delivered
	// The write has failed by now; clear the way and stop well inside the
	// retry interval.
	time.Sleep(100 * time.Millisecond)
	unblock()
	cancel()
	<-finished

	if status, ok, _ := ReadStatus(dir, "api", sent.ID); !ok || status.State != Delivered {
		t.Errorf("status after shutdown = %+v, want delivered", status)
	}
}
