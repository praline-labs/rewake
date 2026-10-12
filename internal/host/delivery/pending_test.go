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

// A question waiting in this session does not swallow an interim report: it
// is announced like any other mail, and the question keeps waiting.
func TestAnInterimReportIsNotTakenByAWaitingQuestion(t *testing.T) {
	dir := stateDir(t)
	marks := state.AnsweringPath(dir, "api")
	if err := os.MkdirAll(marks, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(marks, "q1"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	interim := message("still running")
	interim.Kind, interim.InReplyTo = mail.Interim, []string{"q1"}
	if err := mail.Put(dir, interim); err != nil {
		t.Fatal(err)
	}
	notices := 0
	server := &Server{Dir: dir, Name: "api", Deliver: func(context.Context, mail.Message) mail.Result {
		notices++
		return mail.Result{State: mail.Delivered, Via: "socket"}
	}}
	server.attempts, server.outcomes = map[string]time.Time{}, map[string]mail.Result{}
	server.drain(context.Background())
	if notices != 1 {
		t.Errorf("notices = %d, want the interim report announced", notices)
	}
}
