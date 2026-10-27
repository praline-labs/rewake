package delivery

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
)

// heldFixture is a server whose harness holds every notice it is given and
// later says how each hold ended.
type heldFixture struct {
	dir      string
	task     mail.Message
	server   *Server
	receipts chan mail.Receipt
	opened   chan struct{}
	calls    chan mail.Message
	stop     func()
}

func startHeld(t *testing.T, kind mail.Kind, gated bool) *heldFixture {
	t.Helper()
	return startAnswering(t, kind, gated, mail.Result{State: mail.Held, Via: "socket", Detail: "the session holds the notice"})
}

// startAnswering is startHeld with the harness answering every notice with
// first instead.
func startAnswering(t *testing.T, kind mail.Kind, gated bool, first mail.Result) *heldFixture {
	t.Helper()
	f := &heldFixture{dir: stateDir(t), receipts: make(chan mail.Receipt), opened: make(chan struct{}), calls: make(chan mail.Message, 8)}
	if !gated {
		close(f.opened)
	}
	f.task = message("rerun the smoke")
	f.task.Kind, f.task.FromEpoch, f.task.ToEpoch = kind, "web-epoch", "api-epoch"
	if err := mail.Put(f.dir, f.task); err != nil {
		t.Fatal(err)
	}
	f.server = &Server{
		Dir: f.dir, Name: "api", Epoch: "api-epoch", Receipts: f.receipts, Opened: f.opened,
		Deliver: func(_ context.Context, m mail.Message) mail.Result {
			f.calls <- m
			return first
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		f.server.Serve(ctx)
	}()
	f.stop = func() { cancel(); <-finished }
	t.Cleanup(f.stop)
	return f
}

// until waits for the task's status to reach a state, and returns it.
func (f *heldFixture) until(t *testing.T, want mail.State) mail.Status {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		status, ok, _ := mail.ReadStatus(f.dir, "api", f.task.ID)
		if ok && status.State == want {
			return status
		}
		if time.Now().After(deadline) {
			t.Fatalf("the task never reached %s; it is %+v", want, status)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// announced is the id of the notice the harness was handed.
func (f *heldFixture) announced(t *testing.T) string {
	t.Helper()
	select {
	case m := <-f.calls:
		return m.ID
	case <-time.After(5 * time.Second):
		t.Fatal("nothing was handed to the harness")
		return ""
	}
}

// told waits for the note the sender is sent about the task; the status is
// written before it.
func (f *heldFixture) told(t *testing.T) *mail.Message {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if note := f.undelivered(t); note != nil {
			return note
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil
}

// undelivered is the note the sender was sent about the task, if any.
func (f *heldFixture) undelivered(t *testing.T) *mail.Message {
	t.Helper()
	messages, err := mail.List(f.dir, "web")
	if err != nil {
		return nil
	}
	for _, m := range messages {
		if m.Undelivered != nil && m.Undelivered.ID == f.task.ID {
			return &m
		}
	}
	return nil
}

func TestAHeldTaskIsNotDelivered(t *testing.T) {
	f := startHeld(t, mail.Task, false)
	f.announced(t)
	status := f.until(t, mail.Held)
	if status.Detail != "the session holds the notice" {
		t.Fatalf("the hold lost its reason: %+v", status)
	}
	if must(mail.Answered(f.dir, "api", f.task.ID)) {
		t.Fatal("a held task left the waiting set")
	}
}

func TestAReleasedTaskIsDelivered(t *testing.T) {
	f := startHeld(t, mail.Task, false)
	id := f.announced(t)
	f.until(t, mail.Held)
	f.receipts <- mail.Receipt{ID: id, Result: mail.Result{State: mail.Delivered, Via: "socket", Detail: "released after being held"}}
	if status := f.until(t, mail.Delivered); status.Detail != "released after being held" {
		t.Fatalf("got %+v", status)
	}
	if f.undelivered(t) != nil {
		t.Fatal("the sender of a released task was told it failed")
	}
}

func TestAnExpiredTaskIsFailedAndItsSenderTold(t *testing.T) {
	f := startHeld(t, mail.Task, false)
	id := f.announced(t)
	f.until(t, mail.Held)
	f.receipts <- mail.Receipt{ID: id, Result: mail.Result{State: mail.Failed, Via: "socket", Detail: "it expired unreleased"}}
	f.until(t, mail.Failed)
	note := f.told(t)
	if note == nil || note.Kind != mail.Note || note.To != "web" || note.ToEpoch != "web-epoch" || note.Undelivered.Kind != mail.Task {
		t.Fatalf("the sender was not told: %+v", note)
	}
	if note.Text != "Rewake: your task to api was not delivered: it expired unreleased. Send it again if it still matters.\n\nWhat was sent:\nrerun the smoke" {
		t.Fatalf("the note does not say what or why: %q", note.Text)
	}
	if mail.Owed(*note) {
		t.Fatal("the note itself asks for a report")
	}
}

// A note owes nothing, so its sender is not written to about it.
func TestAnExpiredNoteTellsNobody(t *testing.T) {
	f := startHeld(t, mail.Note, false)
	id := f.announced(t)
	f.until(t, mail.Held)
	f.receipts <- mail.Receipt{ID: id, Result: mail.Result{State: mail.Failed, Detail: "expired"}}
	f.until(t, mail.Failed)
	if f.undelivered(t) != nil {
		t.Fatal("the sender of a note was told")
	}
}

func TestASessionEndingFailsWhatItHolds(t *testing.T) {
	f := startHeld(t, mail.Task, false)
	f.announced(t)
	f.until(t, mail.Held)
	f.stop()
	status, _, _ := mail.ReadStatus(f.dir, "api", f.task.ID)
	if status.State != mail.Failed || !strings.Contains(status.Detail, "held") {
		t.Fatalf("a hold nothing can release now stayed %+v", status)
	}
	if f.told(t) == nil {
		t.Fatal("the sender was not told")
	}
}

// The agent may read a held message on its own; that read is final, and a
// later expiry neither undoes it nor tells the sender it failed.
func TestAHeldTaskReadAnywayStaysRead(t *testing.T) {
	f := startHeld(t, mail.Task, false)
	id := f.announced(t)
	f.until(t, mail.Held)
	readAll(t, f.dir, "api-epoch")
	f.until(t, mail.Read)
	f.receipts <- mail.Receipt{ID: id, Result: mail.Result{State: mail.Failed, Detail: "expired"}}
	time.Sleep(100 * time.Millisecond)
	if status, _, _ := mail.ReadStatus(f.dir, "api", f.task.ID); status.State != mail.Read {
		t.Fatalf("a read task became %+v", status)
	}
	if f.undelivered(t) != nil {
		t.Fatal("the sender of a task that was read was told it failed")
	}
}

// Until the harness can take a notice, mail waits without being made readable,
// and goes out as soon as it opens.
func TestTheFirstNoticeWaitsForTheOpening(t *testing.T) {
	f := startHeld(t, mail.Task, true)
	if status := f.until(t, mail.Pending); status.Detail != opening {
		t.Fatalf("got %+v", status)
	}
	select {
	case <-f.calls:
		t.Fatal("a notice went out before the session could take it")
	case <-time.After(3 * pollInterval):
	}
	if available, _ := mail.Open(f.dir, "api", "api-epoch").AvailableUnread(); len(available) != 0 {
		t.Fatal("mail was readable before it was announced")
	}
	started := time.Now()
	close(f.opened)
	f.announced(t)
	if waited := time.Since(started); waited > time.Second {
		t.Fatalf("the waiting mail went out %v after the opening", waited)
	}
}
