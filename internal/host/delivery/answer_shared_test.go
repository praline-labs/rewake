package delivery

import (
	"context"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
)

func TestAnActiveAnswerDoesNotExpire(t *testing.T) {
	dir := stateDir(t)
	release := reserve(t, dir, "q1")
	defer release()
	report := message("long running question")
	report.Kind = mail.Finished
	report.InReplyTo = []string{"q1"}
	report.CreatedAt = time.Now().Add(-2 * DefaultTTL)
	if err := mail.Put(dir, report); err != nil {
		t.Fatal(err)
	}
	server := &Server{Dir: dir, Name: "api", attempts: map[string]time.Time{}, outcomes: map[string]mail.Result{}, Deliver: func(context.Context, mail.Message) mail.Result {
		t.Fatal("reserved answer announced")
		return mail.Result{}
	}}
	server.drain(context.Background())
	messages, err := mail.PeekUnread(dir, "api", "")
	if err != nil || len(messages) != 1 {
		t.Fatalf("active answer expired: %v %v", messages, err)
	}
}
