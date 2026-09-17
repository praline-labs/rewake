package inbox

import (
	"context"
	"testing"
	"time"
)

func TestStoppedReceiptDoesNotAcknowledgeTheFinalReport(t *testing.T) {
	dir := stateDir(t)
	server := &Server{Dir: dir, Name: "api", attempts: map[string]time.Time{}, outcomes: map[string]Result{}, Deliver: func(context.Context, Message) Result { return Result{State: Delivered} }}
	releaseFirst := reserve(t, dir, "q1")
	stopped := message("the person at the keyboard stopped this turn")
	stopped.Kind = Stopped
	stopped.InReplyTo = []string{"q1"}
	if err := Put(dir, stopped); err != nil {
		t.Fatal(err)
	}
	server.drain(context.Background())
	first, found, err := AwaitAnswer(context.Background(), dir, "api", "", "q1", func(Message) error { return nil })
	if err != nil || !found || first.Kind != Stopped {
		t.Fatalf("stopped=%+v %v %v", first, found, err)
	}
	releaseFirst()
	server.drain(context.Background())
	releaseSecond := reserve(t, dir, "q2")
	defer releaseSecond()
	final := message("human continuation completed both tasks")
	final.Kind = Finished
	final.InReplyTo = []string{"q1", "q2"}
	if err := Put(dir, final); err != nil {
		t.Fatal(err)
	}
	server.drain(context.Background())
	second, found, err := AwaitAnswer(context.Background(), dir, "api", "", "q2", func(Message) error { return nil })
	if err != nil || !found || second.Kind != Finished {
		t.Fatalf("final=%+v %v %v", second, found, err)
	}
	server.drain(context.Background())
	left, err := AvailableUnread(dir, "api", "")
	t.Logf("q1 received stopped, q2 received final; remaining final reports=%d err=%v", len(left), err)
	if err != nil || len(left) != 1 || left[0].ID != final.ID {
		t.Fatal("receipt for stopped consumed q1's later finished outcome")
	}
}

func TestSharedStoppedAndFinalReceiptsRemainIndependent(t *testing.T) {
	dir := stateDir(t)
	server := &Server{Dir: dir, Name: "api", attempts: map[string]time.Time{}, outcomes: map[string]Result{}, Deliver: func(context.Context, Message) Result { return Result{State: Delivered} }}
	first := reserve(t, dir, "q1")
	defer first()
	second := reserve(t, dir, "q2")
	defer second()
	stopped := message("keyboard stop")
	stopped.Kind = Stopped
	stopped.InReplyTo = []string{"q1", "q2"}
	if err := Put(dir, stopped); err != nil {
		t.Fatal(err)
	}
	server.drain(context.Background())
	for _, question := range []string{"q1", "q2"} {
		answer, found, err := AwaitAnswer(context.Background(), dir, "api", "", question, func(Message) error { return nil })
		if err != nil || !found || answer.ID != stopped.ID {
			t.Fatalf("shared stop missing: %s %+v %v", question, answer, err)
		}
	}
	server.drain(context.Background())
	if left, _ := AvailableUnread(dir, "api", ""); len(left) != 0 {
		t.Fatal("shared stopped was not fully acknowledged")
	}
	third := reserve(t, dir, "q3")
	defer third()
	final := message("human continuation")
	final.Kind = Finished
	final.InReplyTo = []string{"q1", "q2", "q3"}
	if err := Put(dir, final); err != nil {
		t.Fatal(err)
	}
	server.drain(context.Background())
	if _, found, err := AwaitAnswer(context.Background(), dir, "api", "", "q3", func(Message) error { return nil }); err != nil || !found {
		t.Fatal("new question did not receive the final result")
	}
	server.drain(context.Background())
	if left, _ := AvailableUnread(dir, "api", ""); len(left) != 1 || left[0].ID != final.ID {
		t.Fatal("stopped consumers lost their later ordinary result")
	}
}
