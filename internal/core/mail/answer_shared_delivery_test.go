package mail

import (
	"context"
	"testing"
	"time"
)

func TestAnActiveAnswerDoesNotExpire(t *testing.T) {
	dir := stateDir(t)
	release := reserve(t, dir, "q1")
	defer release()
	report := message("long running question")
	report.Kind = Finished
	report.InReplyTo = []string{"q1"}
	report.CreatedAt = time.Now().Add(-2 * DefaultTTL)
	if err := Put(dir, report); err != nil {
		t.Fatal(err)
	}
	server := &Server{Dir: dir, Name: "api", attempts: map[string]time.Time{}, outcomes: map[string]Result{}, Deliver: func(context.Context, Message) Result { t.Fatal("reserved answer announced"); return Result{} }}
	server.drain(context.Background())
	messages, err := PeekUnread(dir, "api", "")
	if err != nil || len(messages) != 1 {
		t.Fatalf("active answer expired: %v %v", messages, err)
	}
}
