package delivery

import (
	"context"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
)

func TestStoppedReceiptDoesNotAcknowledgeTheFinalReport(t *testing.T) {
	dir := stateDir(t)
	server := &Server{Dir: dir, Name: "api", attempts: map[string]time.Time{}, outcomes: map[string]mail.Result{}, Deliver: func(context.Context, mail.Message) mail.Result { return mail.Result{State: mail.Delivered} }}
	releaseFirst := reserve(t, dir, "q1")
	stopped := message("the person at the keyboard stopped this turn")
	stopped.Kind = mail.Stopped
	stopped.InReplyTo = []string{"q1"}
	if err := mail.Put(dir, stopped); err != nil {
		t.Fatal(err)
	}
	server.drain(context.Background())
	first, found, err := mail.Open(dir, "api", "").AwaitAnswer(context.Background(), "q1", func(mail.Message) error { return nil })
	if err != nil || !found || first.Kind != mail.Stopped {
		t.Fatalf("stopped=%+v %v %v", first, found, err)
	}
	releaseFirst()
	server.drain(context.Background())
	releaseSecond := reserve(t, dir, "q2")
	defer releaseSecond()
	final := message("human continuation completed both tasks")
	final.Kind = mail.Finished
	final.InReplyTo = []string{"q1", "q2"}
	if err := mail.Put(dir, final); err != nil {
		t.Fatal(err)
	}
	server.drain(context.Background())
	second, found, err := mail.Open(dir, "api", "").AwaitAnswer(context.Background(), "q2", func(mail.Message) error { return nil })
	if err != nil || !found || second.Kind != mail.Finished {
		t.Fatalf("final=%+v %v %v", second, found, err)
	}
	server.drain(context.Background())
	left, err := mail.Open(dir, "api", "").AvailableUnread()
	t.Logf("q1 received stopped, q2 received final; remaining final reports=%d err=%v", len(left), err)
	if err != nil || len(left) != 1 || left[0].ID != final.ID {
		t.Fatal("receipt for stopped consumed q1's later finished outcome")
	}
}

func TestSharedStoppedAndFinalReceiptsRemainIndependent(t *testing.T) {
	dir := stateDir(t)
	server := &Server{Dir: dir, Name: "api", attempts: map[string]time.Time{}, outcomes: map[string]mail.Result{}, Deliver: func(context.Context, mail.Message) mail.Result { return mail.Result{State: mail.Delivered} }}
	first := reserve(t, dir, "q1")
	defer first()
	second := reserve(t, dir, "q2")
	defer second()
	stopped := message("keyboard stop")
	stopped.Kind = mail.Stopped
	stopped.InReplyTo = []string{"q1", "q2"}
	if err := mail.Put(dir, stopped); err != nil {
		t.Fatal(err)
	}
	server.drain(context.Background())
	for _, question := range []string{"q1", "q2"} {
		answer, found, err := mail.Open(dir, "api", "").AwaitAnswer(context.Background(), question, func(mail.Message) error { return nil })
		if err != nil || !found || answer.ID != stopped.ID {
			t.Fatalf("shared stop missing: %s %+v %v", question, answer, err)
		}
	}
	server.drain(context.Background())
	if left, _ := mail.Open(dir, "api", "").AvailableUnread(); len(left) != 0 {
		t.Fatal("shared stopped was not fully acknowledged")
	}
	third := reserve(t, dir, "q3")
	defer third()
	final := message("human continuation")
	final.Kind = mail.Finished
	final.InReplyTo = []string{"q1", "q2", "q3"}
	if err := mail.Put(dir, final); err != nil {
		t.Fatal(err)
	}
	server.drain(context.Background())
	if _, found, err := mail.Open(dir, "api", "").AwaitAnswer(context.Background(), "q3", func(mail.Message) error { return nil }); err != nil || !found {
		t.Fatal("new question did not receive the final result")
	}
	server.drain(context.Background())
	if left, _ := mail.Open(dir, "api", "").AvailableUnread(); len(left) != 1 || left[0].ID != final.ID {
		t.Fatal("stopped consumers lost their later ordinary result")
	}
}
