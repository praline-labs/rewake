package delivery

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/mail/mailtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

func TestAReleasedOldAnswerIsStillAnnounced(t *testing.T) {
	dir := stateDir(t)
	release := reserve(t, dir, "q1")
	report := message("must remain recoverable")
	report.Kind = mail.Finished
	report.InReplyTo = []string{"q1"}
	report.CreatedAt = time.Now().Add(-2 * DefaultTTL)
	if err := mail.Put(dir, report); err != nil {
		t.Fatal(err)
	}
	notices := 0
	server := &Server{Dir: dir, Name: "api", attempts: map[string]time.Time{}, outcomes: map[string]mail.Result{}, Deliver: func(context.Context, mail.Message) mail.Result { notices++; return mail.Result{State: mail.Delivered} }}
	server.drain(context.Background())
	before, _ := mail.PeekUnread(dir, "api", "")
	release()
	server.drain(context.Background())
	after, _ := mail.PeekUnread(dir, "api", "")
	status, _, _ := mail.ReadStatus(dir, "api", report.ID)
	t.Logf("before=%d after=%d notices=%d status=%+v", len(before), len(after), notices, status)
	if len(after) != 1 || notices != 1 {
		t.Fatal("reserved answer discarded after release")
	}
}

func TestAcceptedAnswersSurviveRetentionAndServerRestart(t *testing.T) {
	dir := stateDir(t)
	run := liveRunOf(t, dir, "api")
	release := reserve(t, dir, "q1")
	report := message("old accepted result")
	report.Kind, report.ToEpoch = mail.Finished, run
	report.InReplyTo = []string{"q1"}
	report.CreatedAt = time.Now().Add(-mailtest.PastRetention)
	if err := mail.Put(dir, report); err != nil {
		t.Fatal(err)
	}
	notices := 0
	newServer := func() *Server {
		return &Server{Dir: dir, Name: "api", Epoch: run, attempts: map[string]time.Time{}, outcomes: map[string]mail.Result{}, Deliver: func(context.Context, mail.Message) mail.Result { notices++; return mail.Result{State: mail.Delivered} }}
	}
	server := newServer()
	server.drain(context.Background())
	path := filepath.Join(state.UnreadPath(dir, "api"), report.ID+".json")
	old := time.Now().Add(-mailtest.PastRetention)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	server.sweepFinished()
	release()
	server = newServer()
	server.drain(context.Background())
	server.sweepFinished()
	messages, err := mail.PeekUnread(dir, "api", run)
	if err != nil || len(messages) != 1 || notices != 1 {
		t.Fatalf("accepted report lost: messages=%v notices=%d err=%v", messages, notices, err)
	}
}
