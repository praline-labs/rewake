package delivery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/session"
	"github.com/praline-labs/rewake/internal/infra/boottime"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// stalledServer is api's server after its turn ended pending two seconds ago
// with web's task read and owed, and nothing heard of api since; it delivers
// into notices.
func stalledServer(t *testing.T, notices *[]mail.Message) *Server {
	t.Helper()
	dir := stateDir(t)
	web := liveRunOf(t, dir, "web")
	run := liveRunOf(t, dir, "api")
	self := must(registry.Lookup(dir, "api"))
	task := mail.Message{ID: mail.NewID(), From: "web", FromEpoch: web, To: "api", ToEpoch: run, Kind: mail.Task, Text: "run the suite", CreatedAt: time.Now()}
	if err := mail.Put(dir, task); err != nil {
		t.Fatal(err)
	}
	readIn(t, dir, run, "t", task)
	ended := boottime.Now() - int64(2*time.Second)
	if err := withLock(dir, "api", func() error {
		return mail.MarkPending(dir, "api", run, mail.MarkName(ended-1, mail.NewID()), "the suite runs", ended-1)
	}); err != nil {
		t.Fatal(err)
	}
	if outcome, err := mail.CompleteTurn(t.Context(), dir, self, mail.TurnEnd{Text: "started it", Started: ended - 2, Ended: ended}, "t", mail.TurnEndOptions{}); err != nil || !outcome.Interim {
		t.Fatalf("the pending end: %+v %v", outcome, err)
	}
	s := batchServer(dir)
	s.Epoch, s.Stall = run, time.Second
	s.Deliver = func(_ context.Context, m mail.Message) mail.Result {
		*notices = append(*notices, m)
		return mail.Result{State: mail.Delivered, Via: "socket"}
	}
	return s
}

// stallLetters are the stall reminders api's mailbox holds, waiting or unread.
func stallLetters(t *testing.T, s *Server) []mail.Message {
	t.Helper()
	var found []mail.Message
	waiting := must(mail.List(s.Dir, "api"))
	unread := must(mail.Open(s.Dir, "api", s.Epoch).PeekUnread())
	for _, m := range append(waiting, unread...) {
		if m.From == "api" && strings.HasPrefix(m.Text, "Rewake: your pending ") {
			found = append(found, m)
		}
	}
	return found
}

// A worker idle under a pending mark past the bound, its sender still
// waiting, is woken once: a notify of its own, announced as any other mail.
// Not before the bound, not while its telemetry does not call it idle, not
// for a run whose turns are reported to nobody, and not again — by the same
// server or by the next image's.
func TestAStalledWorkerIsWokenOnce(t *testing.T) {
	var notices []mail.Message
	s := stalledServer(t, &notices)
	ctx := context.Background()
	idle := false
	s.StallSnapshot = func() session.Snapshot {
		activity := session.ActivityWorking
		if idle {
			activity = session.ActivityIdle
		}
		return freshStallSnapshot(activity)
	}

	s.remindStalled(ctx)
	if got := stallLetters(t, s); len(got) != 0 {
		t.Fatalf("a worker its telemetry calls busy was reminded: %+v", got)
	}
	idle = true
	s.Stall = time.Hour
	s.remindStalled(ctx)
	if got := stallLetters(t, s); len(got) != 0 {
		t.Fatalf("a worker idle for less than the bound was reminded: %+v", got)
	}
	s.Stall = 0
	s.remindStalled(ctx)
	if got := stallLetters(t, s); len(got) != 0 {
		t.Fatalf("a run with no bound was reminded: %+v", got)
	}

	s.Stall = time.Second
	s.remindStalled(ctx)
	got := stallLetters(t, s)
	if len(got) != 1 || got[0].Kind != mail.Note || !strings.Contains(got[0].Text, `"the suite runs"`) || !strings.Contains(got[0].Text, "task of web") {
		t.Fatalf("reminders after the bound: %+v", got)
	}
	s.drain(ctx)
	if len(notices) != 1 || notices[0].ID != got[0].ID {
		t.Fatalf("notices %+v, want the reminder announced", notices)
	}

	s.remindStalled(ctx)
	next := batchServer(s.Dir)
	next.Epoch, next.Stall, next.StallSnapshot = s.Epoch, time.Second, s.StallSnapshot
	next.remindStalled(ctx)
	if got := stallLetters(t, s); len(got) != 1 {
		t.Fatalf("%d reminders after a second look and a new image's, want one", len(got))
	}
}

// An unusable lock is not contention: publication and its second live sample
// must not run lockless even when the reminder records remain writable.
func TestAStallWithoutAUsableLockPublishesNothing(t *testing.T) {
	var notices []mail.Message
	s := stalledServer(t, &notices)
	looks := 0
	s.StallSnapshot = func() session.Snapshot {
		looks++
		return freshStallSnapshot(session.ActivityIdle)
	}
	lock := filepath.Join(state.InboxPath(s.Dir, s.Name), ".lock")
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(lock, 0o700); err != nil {
		t.Fatal(err)
	}
	s.remindStalled(context.Background())
	if looks != 1 || len(stallLetters(t, s)) != 0 || s.reminded != "" {
		t.Fatalf("lockless rejudgment/publication: samples %d, letters %+v, remembered %q", looks, stallLetters(t, s), s.reminded)
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	s.remindStalled(context.Background())
	if len(stallLetters(t, s)) != 1 || s.reminded == "" {
		t.Fatal("a usable lock did not permit the reminder")
	}
}

// What lands between the pass's first look and its lock — the report that
// settles the task, a turn's start — is what the reminder is judged by: the
// look without the lock decides nothing.
func TestAStallIsJudgedAgainUnderTheLock(t *testing.T) {
	for name, between := range map[string]func(t *testing.T, s *Server){
		"the report settles the task": func(t *testing.T, s *Server) {
			self := must(registry.Lookup(s.Dir, s.Name))
			if _, err := mail.CompleteTurn(context.Background(), s.Dir, self, mail.TurnEnd{Text: "done", Started: boottime.Now() - 1000, Ended: boottime.Now()}, "t", mail.TurnEndOptions{}); err != nil {
				t.Fatal(err)
			}
		},
		"a turn starts": func(t *testing.T, s *Server) {
			if err := mail.Open(s.Dir, s.Name, s.Epoch).RecordTurnStart(boottime.Now()); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			var notices []mail.Message
			s := stalledServer(t, &notices)
			s.StallSnapshot = func() session.Snapshot { return freshStallSnapshot(session.ActivityIdle) }
			beforeStallLock(t, s, func() { between(t, s) })
			if s.reminded != "" {
				t.Fatalf("remembered an unpublished reminder: %s", s.reminded)
			}
			if got := stallLetters(t, s); len(got) != 0 {
				t.Fatalf("reminded after %s: %+v", name, got)
			}
		})
	}
}
