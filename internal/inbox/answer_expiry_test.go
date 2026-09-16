package inbox

import (
	"context"
	"github.com/iiiokojiadbi/rewake/internal/state"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAReleasedOldAnswerIsStillAnnounced(t *testing.T) {
	dir := stateDir(t)
	release := reserve(t, dir, "q1")
	report := message("must remain recoverable")
	report.Kind = Finished
	report.InReplyTo = []string{"q1"}
	report.CreatedAt = time.Now().Add(-2 * defaultTTL)
	if err := Put(dir, report); err != nil {
		t.Fatal(err)
	}
	notices := 0
	server := &Server{Dir: dir, Name: "api", attempts: map[string]time.Time{}, outcomes: map[string]Result{}, Deliver: func(context.Context, Message) Result { notices++; return Result{State: Delivered} }}
	server.drain(context.Background())
	before, _ := PeekUnread(dir, "api", "")
	release()
	server.drain(context.Background())
	after, _ := PeekUnread(dir, "api", "")
	status, _ := ReadStatus(dir, "api", report.ID)
	t.Logf("before=%d after=%d notices=%d status=%+v", len(before), len(after), notices, status)
	if len(after) != 1 || notices != 1 {
		t.Fatal("reserved answer discarded after release")
	}
}

func TestAcceptedAnswersSurviveRetentionAndServerRestart(t *testing.T) {
	dir := stateDir(t)
	release := reserve(t, dir, "q1")
	report := message("old accepted result")
	report.Kind = Finished
	report.InReplyTo = []string{"q1"}
	report.CreatedAt = time.Now().Add(-2 * keepFinished)
	if err := Put(dir, report); err != nil {
		t.Fatal(err)
	}
	notices := 0
	newServer := func() *Server {
		return &Server{Dir: dir, Name: "api", attempts: map[string]time.Time{}, outcomes: map[string]Result{}, Deliver: func(context.Context, Message) Result { notices++; return Result{State: Delivered} }}
	}
	server := newServer()
	server.drain(context.Background())
	path := filepath.Join(state.UnreadPath(dir, "api"), report.ID+".json")
	old := time.Now().Add(-2 * keepFinished)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	server.sweepFinished()
	release()
	server = newServer()
	server.drain(context.Background())
	server.sweepFinished()
	messages, err := PeekUnread(dir, "api", "")
	if err != nil || len(messages) != 1 || notices != 1 {
		t.Fatalf("accepted report lost: messages=%v notices=%d err=%v", messages, notices, err)
	}
}
