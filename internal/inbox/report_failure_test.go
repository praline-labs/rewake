package inbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

func reportFailureFixture(t *testing.T) (*Server, Message, *int) {
	t.Helper()
	dir := stateDir(t)
	m := message("retained result")
	m.Kind, m.ToEpoch, m.FromEpoch = Finished, "1.1", "2.2"
	if err := Put(dir, m); err != nil {
		t.Fatal(err)
	}
	calls := 0
	s := &Server{Dir: dir, Name: m.To, Epoch: m.ToEpoch, attempts: map[string]time.Time{}, outcomes: map[string]Result{}, Deliver: func(context.Context, Message) Result {
		calls++
		return Result{State: Failed, Detail: "notification refused"}
	}}
	return s, m, &calls
}

func requireReportState(t *testing.T, s *Server, m Message, want State, visible bool) {
	t.Helper()
	status, ok := ReadStatus(s.Dir, s.Name, m.ID)
	if !ok || status.State != want {
		t.Fatalf("status=%+v want=%s", status, want)
	}
	available, err := AvailableUnread(s.Dir, s.Name, s.Epoch)
	count := 0
	if visible {
		count = 1
	}
	if err != nil || len(available) != count {
		t.Fatalf("unread=%v err=%v", available, err)
	}
	if want == Failed && visible && (!status.ReportAvailable || status.Detail == "") {
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
				m.Kind = Task
			case "notify":
				m.Kind = Note
			case "read during failure":
				s.Deliver = func(context.Context, Message) Result {
					if err := MarkRead(s.Dir, s.Name, s.Epoch, m, true); err != nil {
						t.Fatal(err)
					}
					return Result{State: Failed, Detail: "late refusal"}
				}
			}
			if err := Put(s.Dir, m); err != nil {
				t.Fatal(err)
			}
			if mode == "expired after pending" {
				if err := linkUnread(s.Dir, s.Name, m.ID); err != nil {
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
				if _, ok := ReadStatus(s.Dir, s.Name, m.ID); ok {
					t.Fatal("shutdown changed foreign mail")
				}
				return
			}
			want := Failed
			if mode == "read during failure" {
				want = Read
			}
			requireReportState(t, s, m, want, mode == "shutdown")
			if mode == "expired" && *calls != 0 {
				t.Fatal("expired report announced")
			}
		})
	}
}

func TestReportFailureRecovery(t *testing.T) {
	for _, fault := range []string{"status", "removal", "archive"} {
		t.Run(fault, func(t *testing.T) {
			s, m, calls := reportFailureFixture(t)
			var unblock func()
			switch fault {
			case "status":
				path := statusPath(s.Dir, s.Name, m.ID)
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
				unblock = func() {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				}
			case "removal":
				original := removeWaiting
				removeWaiting = func(string) error { return os.ErrPermission }
				t.Cleanup(func() { removeWaiting = original })
				unblock = func() { removeWaiting = original }
			case "archive":
				if err := os.WriteFile(state.DonePath(s.Dir, s.Name), nil, 0o600); err != nil {
					t.Fatal(err)
				}
				unblock = func() {}
			}
			s.drain(context.Background())
			if unread, err := PeekUnread(s.Dir, s.Name, s.Epoch); err != nil || len(unread) != 1 {
				t.Fatalf("fault lost report: %v %v", unread, err)
			}
			unblock()
			if fault == "status" {
				s.refuseWaiting("session ended")
			} else {
				// Recreate volatile state: durable failed status must prevent another RPC.
				s.attempts = map[string]time.Time{}
				s.outcomes = map[string]Result{}
				s.drain(context.Background())
			}
			requireReportState(t, s, m, Failed, true)
			if *calls != 1 {
				t.Fatalf("failure retried %d times", *calls)
			}
			if queued, _ := list(s.Dir, s.Name); len(queued) != 0 {
				t.Fatal("report still queued")
			}
			if fault == "archive" {
				if err := MarkRead(s.Dir, s.Name, s.Epoch, m, true); err == nil {
					t.Fatal("archive obstruction ignored")
				}
				if unread, _ := PeekUnread(s.Dir, s.Name, s.Epoch); len(unread) != 1 {
					t.Fatal("failed read removed report")
				}
				if err := os.Remove(state.DonePath(s.Dir, s.Name)); err != nil {
					t.Fatal(err)
				}
				if err := MarkRead(s.Dir, s.Name, s.Epoch, m, true); err != nil {
					t.Fatal(err)
				}
				requireReportState(t, s, m, Read, false)
			}
		})
	}
}

func TestFailedReportReservationAndRetention(t *testing.T) {
	s, m, calls := reportFailureFixture(t)
	m.InReplyTo = []string{"q1", "q2"}
	m.CreatedAt = time.Now().Add(-2 * DefaultTTL)
	if err := Put(s.Dir, m); err != nil {
		t.Fatal(err)
	}
	release := reserve(t, s.Dir, "q1")
	s.drain(context.Background())
	s.refuseWaiting("session ended")
	if available, _ := AvailableUnread(s.Dir, s.Name, s.Epoch); len(available) != 0 || *calls != 0 {
		t.Fatal("reserved report escaped")
	}
	// A stopped receipt must not consume the later finished result.
	if err := state.EnsureSubdir(answerReceiptsPath(s.Dir, s.Name)); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteAtomic(filepath.Join(answerReceiptsPath(s.Dir, s.Name), "q2"), []byte("earlier-stopped-report")); err != nil {
		t.Fatal(err)
	}
	release()
	s.stopping = false
	s.lockContext = context.Background()
	s.drain(context.Background())
	requireReportState(t, s, m, Failed, true)
	if *calls != 1 {
		t.Fatalf("notice calls=%d", *calls)
	}
	if err := receiveAnswer(s.Dir, s.Name, s.Epoch, "q1", m); err != nil {
		t.Fatal(err)
	}
	requireReportState(t, s, m, Failed, true)
	s.sweepFinished()
	requireReportState(t, s, m, Failed, true)
	old := time.Now().Add(-2 * keepFinished)
	if err := os.Chtimes(filepath.Join(state.UnreadPath(s.Dir, s.Name), m.ID+".json"), old, old); err != nil {
		t.Fatal(err)
	}
	s.sweepFinished()
	if unread, _ := PeekUnread(s.Dir, s.Name, s.Epoch); len(unread) != 0 {
		t.Fatal("normal retention no longer expires report")
	}
}
