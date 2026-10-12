package delivery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/mail/mailtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

func TestReceiptsOutliveTheirSharedAnswer(t *testing.T) {
	dir := stateDir(t)
	run := liveRunOf(t, dir, "api")
	first, second := reserve(t, dir, "q1"), reserve(t, dir, "q2")
	defer first()
	defer second()
	report := message("shared result")
	report.Kind, report.ToEpoch = mail.Finished, run
	report.InReplyTo = []string{"q1", "q2"}
	if err := mail.Put(dir, report); err != nil {
		t.Fatal(err)
	}
	notices := 0
	server := &Server{Dir: dir, Name: "api", Epoch: run, attempts: map[string]time.Time{}, outcomes: map[string]mail.Result{}, Deliver: func(context.Context, mail.Message) mail.Result { notices++; return mail.Result{State: mail.Delivered} }}
	server.drain(context.Background())
	if _, found, err := mail.AwaitAnswer(context.Background(), dir, "api", run, "q1", func(mail.Message) error { return nil }); err != nil || !found {
		t.Fatalf("first: %v %v", found, err)
	}
	receipt := filepath.Join(mailtest.AnswerReceiptsPath(dir, "api"), "q1")
	old := time.Now().Add(-mailtest.PastRetention)
	if err := os.Chtimes(receipt, old, old); err != nil {
		t.Fatal(err)
	}
	server.sweepFinished()
	if _, found, err := mail.AwaitAnswer(context.Background(), dir, "api", run, "q2", func(mail.Message) error { return nil }); err != nil || !found {
		t.Fatalf("second: %v %v", found, err)
	}
	server.drain(context.Background())
	left, _ := mail.PeekUnread(dir, "api", run)
	t.Logf("both outputs succeeded; unread=%d notices=%d", len(left), notices)
	if len(left) != 0 || notices != 0 {
		t.Fatal("shared report is announced again after both questions received it")
	}
}

func TestUnreservedReportsKeepTheirExpiry(t *testing.T) {
	dir := stateDir(t)
	run := liveRunOf(t, dir, "api")
	report := message("never reserved")
	report.Kind, report.ToEpoch = mail.Finished, run
	if err := mail.Put(dir, report); err != nil {
		t.Fatal(err)
	}
	attempts := 0
	server := &Server{Dir: dir, Name: "api", Epoch: run, attempts: map[string]time.Time{}, outcomes: map[string]mail.Result{}, Deliver: func(context.Context, mail.Message) mail.Result { attempts++; return mail.Result{State: mail.Pending} }}
	server.drain(context.Background())
	path := filepath.Join(state.InboxPath(dir, "api"), report.ID+".json")
	old := time.Now().Add(-mailtest.PastRetention)
	// Age both the hard-linked queue and readable copy without waiting two days.
	report.CreatedAt = old
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{path, filepath.Join(state.UnreadPath(dir, "api"), report.ID+".json")} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	server.attempts = map[string]time.Time{}
	server.drain(context.Background())
	server.sweepFinished()
	left, _ := mail.PeekUnread(dir, "api", run)
	status, _, _ := mail.ReadStatus(dir, "api", report.ID)
	t.Logf("no reservation ever; attempts=%d unread=%d status=%s", attempts, len(left), status.State)
	if status.State != mail.Failed {
		t.Fatal("unreserved undelivered report has no expiry and refreshes its retention")
	}
}

func TestReleasedAnswerRetriesCannotRenewTheirDeadline(t *testing.T) {
	dir := stateDir(t)
	release := reserve(t, dir, "q")
	answer := message("old result")
	answer.Kind = mail.Finished
	answer.InReplyTo = []string{"q"}
	answer.CreatedAt = time.Now().Add(-2 * DefaultTTL)
	if err := mail.Put(dir, answer); err != nil {
		t.Fatal(err)
	}
	server := &Server{Dir: dir, Name: "api", attempts: map[string]time.Time{}, outcomes: map[string]mail.Result{}, Deliver: func(context.Context, mail.Message) mail.Result { return mail.Result{State: mail.Pending} }}
	server.drain(context.Background())
	release()
	server.drain(context.Background())
	path := filepath.Join(mail.RetentionPath(dir, "api"), answer.ID)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	server.attempts = map[string]time.Time{}
	server.drain(context.Background())
	again, err := os.ReadFile(path)
	if err != nil || string(raw) != string(again) {
		t.Fatalf("retry renewed deadline: %s %s %v", raw, again, err)
	}
	expired, _ := json.Marshal(mail.AnswerLifetime{ReleasedAt: time.Now().Add(-2 * DefaultTTL)})
	if err := os.WriteFile(path, expired, 0o600); err != nil {
		t.Fatal(err)
	}
	server.attempts = map[string]time.Time{}
	server.drain(context.Background())
	status, _, _ := mail.ReadStatus(dir, "api", answer.ID)
	if status.State != mail.Failed {
		t.Fatalf("released answer never expired: %+v", status)
	}
}
