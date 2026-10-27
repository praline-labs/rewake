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

func reportFailureFixture(t *testing.T) (*Server, mail.Message, *int) {
	t.Helper()
	dir := stateDir(t)
	m := message("retained result")
	m.Kind, m.ToEpoch, m.FromEpoch = mail.Finished, liveRunOf(t, dir, "api"), "2.2"
	if err := mail.Put(dir, m); err != nil {
		t.Fatal(err)
	}
	calls := 0
	s := &Server{Dir: dir, Name: m.To, Epoch: m.ToEpoch, attempts: map[string]time.Time{}, outcomes: map[string]mail.Result{}, Deliver: func(context.Context, mail.Message) mail.Result {
		calls++
		return mail.Result{State: mail.Failed, Detail: "notification refused"}
	}}
	return s, m, &calls
}

func requireReportState(t *testing.T, s *Server, m mail.Message, want mail.State, visible bool) {
	t.Helper()
	status, ok, _ := mail.ReadStatus(s.Dir, s.Name, m.ID)
	if !ok || status.State != want {
		t.Fatalf("status=%+v want=%s", status, want)
	}
	available, err := mail.Open(s.Dir, s.Name, s.Epoch).AvailableUnread()
	count := 0
	if visible {
		count = 1
	}
	if err != nil || len(available) != count {
		t.Fatalf("unread=%v err=%v", available, err)
	}
	if want == mail.Failed && visible && (!status.ReportAvailable || status.Detail == "") {
		t.Fatalf("missing retained failure diagnostics: %+v", status)
	}
}

func TestReportFailureAdmissionAndShutdown(t *testing.T) {
	for _, mode := range []string{"shutdown", "expired", "expired after pending", "expired shutdown", "foreign", "foreign shutdown", "task", "notify", "read during failure"} {
		t.Run(mode, func(t *testing.T) {
			s, m, calls := reportFailureFixture(t)
			switch mode {
			case "expired", "expired after pending", "expired shutdown":
				m.CreatedAt = time.Now().Add(-2 * DefaultTTL)
			case "foreign", "foreign shutdown":
				m.ToEpoch = "old.1"
			case "task":
				m.Kind = mail.Task
			case "notify":
				m.Kind = mail.Note
			case "read during failure":
				s.Deliver = func(context.Context, mail.Message) mail.Result {
					if err := mail.Open(s.Dir, s.Name, s.Epoch).MarkRead(m, true); err != nil {
						t.Fatal(err)
					}
					return mail.Result{State: mail.Failed, Detail: "late refusal"}
				}
			}
			if err := mail.Put(s.Dir, m); err != nil {
				t.Fatal(err)
			}
			if mode == "expired after pending" {
				if err := mail.LinkUnread(s.Dir, s.Name, m.ID); err != nil {
					t.Fatal(err)
				}
			}
			switch mode {
			case "shutdown", "expired shutdown", "foreign shutdown":
				s.refuseWaiting("session ended")
			case "foreign":
				s.sweepForeign()
			default:
				s.drain(context.Background())
			}
			if mode == "foreign shutdown" {
				if _, ok, _ := mail.ReadStatus(s.Dir, s.Name, m.ID); ok {
					t.Fatal("shutdown changed foreign mail")
				}
				return
			}
			want := mail.Failed
			if mode == "read during failure" {
				want = mail.Read
			}
			requireReportState(t, s, m, want, mode == "shutdown")
			if mode == "expired" && *calls != 0 {
				t.Fatal("expired report announced")
			}
		})
	}
}

// A failed removal of the waiting copy is the same recovery under the
// rewaketest tag, in TestReportFailureRecoveryAfterAFailedRemoval.
func TestReportFailureRecovery(t *testing.T) {
	for _, fault := range []string{"status", "archive"} {
		t.Run(fault, func(t *testing.T) {
			s, m, calls := reportFailureFixture(t)
			var unblock func()
			switch fault {
			case "status":
				unblock = failStatusWrites(t, s.Dir, s.Name, m.ID)
			case "archive":
				if err := os.WriteFile(state.DonePath(s.Dir, s.Name), nil, 0o600); err != nil {
					t.Fatal(err)
				}
				unblock = func() {}
			}
			s.drain(context.Background())
			if unread, err := mail.Open(s.Dir, s.Name, s.Epoch).PeekUnread(); err != nil || len(unread) != 1 {
				t.Fatalf("fault lost report: %v %v", unread, err)
			}
			unblock()
			if fault == "status" {
				s.refuseWaiting("session ended")
			} else {
				// Recreate volatile state: durable failed status must prevent another RPC.
				s.attempts = map[string]time.Time{}
				s.outcomes = map[string]mail.Result{}
				s.drain(context.Background())
			}
			requireReportState(t, s, m, mail.Failed, true)
			if *calls != 1 {
				t.Fatalf("failure retried %d times", *calls)
			}
			if queued, _ := mail.List(s.Dir, s.Name); len(queued) != 0 {
				t.Fatal("report still queued")
			}
			if fault == "archive" {
				if err := mail.Open(s.Dir, s.Name, s.Epoch).MarkRead(m, true); err == nil {
					t.Fatal("archive obstruction ignored")
				}
				if unread, _ := mail.Open(s.Dir, s.Name, s.Epoch).PeekUnread(); len(unread) != 1 {
					t.Fatal("failed read removed report")
				}
				if err := os.Remove(state.DonePath(s.Dir, s.Name)); err != nil {
					t.Fatal(err)
				}
				if err := mail.Open(s.Dir, s.Name, s.Epoch).MarkRead(m, true); err != nil {
					t.Fatal(err)
				}
				requireReportState(t, s, m, mail.Read, false)
			}
		})
	}
}

func TestFailedReportReservationAndRetention(t *testing.T) {
	s, m, calls := reportFailureFixture(t)
	m.InReplyTo = []string{"q1", "q2"}
	m.CreatedAt = time.Now().Add(-2 * DefaultTTL)
	if err := mail.Put(s.Dir, m); err != nil {
		t.Fatal(err)
	}
	release := reserve(t, s.Dir, "q1")
	s.drain(context.Background())
	s.refuseWaiting("session ended")
	if available, _ := mail.Open(s.Dir, s.Name, s.Epoch).AvailableUnread(); len(available) != 0 || *calls != 0 {
		t.Fatal("reserved report escaped")
	}
	// A stopped receipt must not consume the later finished result.
	if err := state.EnsureSubdir(mailtest.AnswerReceiptsPath(s.Dir, s.Name)); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteAtomic(filepath.Join(mailtest.AnswerReceiptsPath(s.Dir, s.Name), "q2"), []byte("earlier-stopped-report")); err != nil {
		t.Fatal(err)
	}
	release()
	s.stopping = false
	s.lockContext = context.Background()
	s.drain(context.Background())
	requireReportState(t, s, m, mail.Failed, true)
	if *calls != 1 {
		t.Fatalf("notice calls=%d", *calls)
	}
	// The asker of q1 takes its answer, which records its receipt.
	if answer, found, err := mail.Open(s.Dir, s.Name, s.Epoch).AwaitAnswer(context.Background(), "q1", func(mail.Message) error { return nil }); err != nil || !found || answer.ID != m.ID {
		t.Fatalf("q1's answer was not taken: %+v %v %v", answer, found, err)
	}
	requireReportState(t, s, m, mail.Failed, true)
	s.sweepFinished()
	requireReportState(t, s, m, mail.Failed, true)
	old := time.Now().Add(-mailtest.PastRetention)
	if err := os.Chtimes(filepath.Join(state.UnreadPath(s.Dir, s.Name), m.ID+".json"), old, old); err != nil {
		t.Fatal(err)
	}
	s.sweepFinished()
	if unread, _ := mail.Open(s.Dir, s.Name, s.Epoch).PeekUnread(); len(unread) != 0 {
		t.Fatal("normal retention no longer expires report")
	}
}
