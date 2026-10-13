package mail

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/registry/registrytest"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// matrixScene is one world's room: api's turn end owes web the report R,
// lead is the room's main, and the world's faults are played through its
// matrixAccess. web's run r is this very process when it decides verified —
// only a run's own wrapper writes as it — and a process of its own
// otherwise, so its end is proven; lead is always a process of its own.
type matrixScene struct {
	t      *testing.T
	dir    string
	access *matrixAccess
	epoch  string
	r      *matrixRun
	lead   *endableRun
	report Message
	// faults are the evidence faults in place, by name.
	unsearchable, effectMet bool
	// initialLanding is how many times R was written before D: its truth.
	initialLanding int
	moments        []matrixMoment
	// views are the decisions as the checks read them, once the world's
	// schedule is over and nothing changes them.
	views []DecisionView
}

// matrixRun is r: served by this process (child nil) or by one of its own.
type matrixRun struct {
	session registry.Session
	child   *endableRun
	ended   bool
}

func (r *matrixRun) epoch() string { return r.session.Epoch() }

// newMatrixScene builds the room with r served by this process when self.
func newMatrixScene(t *testing.T, self bool) *matrixScene {
	t.Helper()
	dir := buildtest.Room(t, matrixBase(t), state.DefaultRoom)
	s := &matrixScene{t: t, dir: dir, epoch: "api-epoch"}
	s.lead = startRoleRun(t, dir, "lead", role.Main.ID)
	s.r = s.startR(self)
	s.access = newMatrixAccess(dir)
	t.Cleanup(func() { testAccess.Delete(dir) })
	s.report = s.owe(s.r)
	return s
}

// memoryFS is where a world's room goes when the system has a file system in
// memory: every durable write of a room flushes, and the matrix makes tens of
// thousands of them, which a disk turns into minutes. A world behaves the same
// on either; only its speed differs.
const memoryFS = "/dev/shm"

// matrixBase is a private base for one world's room.
func matrixBase(t *testing.T) string {
	t.Helper()
	if info, err := os.Stat(memoryFS); err == nil && info.IsDir() {
		if base, err := os.MkdirTemp(memoryFS, "rewake-matrix-"); err == nil {
			t.Cleanup(func() { _ = os.RemoveAll(base) })
			return base
		}
	}
	base := t.TempDir()
	// A base is private, as a session's is; a test directory is not.
	if err := os.Chmod(base, 0o700); err != nil {
		t.Fatal(err)
	}
	return base
}

// startR publishes a run of web.
func (s *matrixScene) startR(self bool) *matrixRun {
	s.t.Helper()
	if !self {
		child := startRoleRun(s.t, s.dir, "web", "")
		return &matrixRun{session: child.session, child: child}
	}
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		s.t.Fatal(err)
	}
	session := registry.Session{Name: "web", ServicePID: os.Getpid(), ServiceStart: start, Boot: registrytest.Boot(s.t), CWD: s.dir, StartedAt: time.Now()}
	if err := registry.Publish(s.dir, session); err != nil {
		s.t.Fatal(err)
	}
	return &matrixRun{session: session}
}

// owe has api's journal of the end owe run a report answering a question.
func (s *matrixScene) owe(run *matrixRun) Message {
	s.t.Helper()
	question := NewID()
	if err := markAwaiting(s.dir, "api", s.epoch, "web", run.epoch(), question); err != nil {
		s.t.Fatal(err)
	}
	waiters, err := ReadWaiters(s.dir, "api", s.epoch)
	if err != nil || len(waiters) != 1 {
		s.t.Fatalf("the wait: %v %v", waiters, err)
	}
	report := Message{ID: ReportID("api", s.epoch, waiters[0]), From: "api", FromEpoch: s.epoch, To: "web", ToEpoch: run.epoch(), Kind: Finished, Text: "the answer", InReplyTo: []string{question}, CreatedAt: time.Now()}
	if err := WriteJournal(s.dir, "api", "end", TurnJournal{Epoch: s.epoch, Reports: []Message{report}, Clear: waiters}); err != nil {
		s.t.Fatal(err)
	}
	return report
}

// startRoleRun is startRun with a role.
func startRoleRun(t *testing.T, dir, name, roleID string) *endableRun {
	t.Helper()
	child := exec.Command("sleep", "120")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	start, err := proc.StartTime(child.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	run := &endableRun{child: child, session: registry.Session{Name: name, Role: roleID, ServicePID: child.Process.Pid, ServiceStart: start, Boot: registrytest.Boot(t), CWD: dir, StartedAt: time.Now()}}
	t.Cleanup(run.end)
	if err := registry.Publish(dir, run.session); err != nil {
		t.Fatal(err)
	}
	return run
}

// endR ends r: its process stops, or, served by this process, its record
// goes as a run's exit takes it.
func (s *matrixScene) endR() {
	s.t.Helper()
	if s.r.ended {
		return
	}
	s.r.ended = true
	if s.r.child != nil {
		s.r.child.end()
		return
	}
	// Without the name's lock: a world may end r inside a mailbox section.
	if err := os.Remove(state.SessionPath(s.dir, "web")); err != nil {
		s.t.Fatal(err)
	}
}

// newRunSweeps starts web's next run and has it sweep, as a server does:
// the marks of every other run go, but those a stop keeps.
func (s *matrixScene) newRunSweeps() {
	s.t.Helper()
	next := startRoleRun(s.t, s.dir, "web", "")
	sweepAsLiveRun(s.t, s.dir, "web", next.session.Epoch())
}

// markPath is R's publication mark.
func (s *matrixScene) markPath(report Message) string {
	path, _ := oncePath(s.dir, report.To, report.ToEpoch, report.ID)
	return path
}

// land writes report into web's mailbox, as an attempt of J that wrote it
// did; with the mark it left — intent, as one stopped before it saved.
func (s *matrixScene) land(report Message, mark string) {
	s.t.Helper()
	if err := PutOnce(s.dir, report); err != nil {
		s.t.Fatal(err)
	}
	s.writeMark(report, mark)
}

func (s *matrixScene) writeMark(report Message, mark string) {
	s.t.Helper()
	path := s.markPath(report)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		s.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(mark), 0o600); err != nil {
		s.t.Fatal(err)
	}
}

// fault puts the evidence faults in place: web's mailbox cannot be searched
// for any letter, by any pass; or the place of report holds what is no
// letter, seen only by the pass that writes.
func (s *matrixScene) fault(unsearchable, effectMet bool, report Message) {
	s.unsearchable, s.effectMet = unsearchable, effectMet
	stages := []string{state.InboxPath(s.dir, "web"), state.UnreadPath(s.dir, "web"), state.DonePath(s.dir, "web")}
	s.access.mu.Lock()
	defer s.access.mu.Unlock()
	if !unsearchable && !effectMet {
		s.access.stat = nil
		return
	}
	s.access.stat = func(path string, plan bool) (fs.FileInfo, error) {
		for _, stage := range stages {
			if filepath.Dir(path) != stage || !strings.HasSuffix(path, ".json") {
				continue
			}
			switch {
			case unsearchable:
				return nil, &fs.PathError{Op: "stat", Path: path, Err: syscall.EIO}
			case effectMet && !plan && filepath.Base(path) == report.ID+".json":
				return notRegular{filepath.Base(path)}, nil
			}
		}
		return nil, nil
	}
}

// barrier is a barrier of api's mailbox.
func (s *matrixScene) barrier() error {
	return state.WithMailboxLock(context.Background(), s.dir, "api", func() error {
		return Reconcile(context.Background(), s.dir, "api")
	})
}

// decide asks for a decision on report, as r verified or as the person.
func (s *matrixScene) decide(report Message, choice string, verified bool) (DecideAnswer, error) {
	by := DecidedBy{Kind: ByPerson}
	if verified {
		by = DecidedBy{Kind: ByRecipient, Name: "web", Run: report.ToEpoch, Verified: true}
	}
	subject, err := FindSubject(s.dir, "api", report.ID)
	if err != nil {
		return DecideAnswer{}, err
	}
	return Decide(context.Background(), s.dir, DecideRequest{Sender: "api", Report: report.ID, Choice: choice, By: by, Expect: subject})
}

// product runs one command of the schedule armed: its durable steps are
// logged and may be cut. When the cut fired in it, the stopped process is
// gone and the same command runs again in one that is not — the barrier
// again, or the same words of the deciding command — so the rest of the
// world's schedule meets what a clean run would.
func (s *matrixScene) product(fn func() error) error {
	s.access.arm(true)
	defer s.access.arm(false)
	err := fn()
	if s.access.lift() {
		err = fn()
	}
	return err
}

// readR has r read report, if it is waiting, and lets enough time pass that
// a sweep may retire it.
func (s *matrixScene) readR(report Message) {
	s.t.Helper()
	if err := archive(s.dir, "web", report.ID); err != nil {
		s.t.Fatal(err)
	}
	read := filepath.Join(state.DonePath(s.dir, "web"), report.ID+".json")
	old := time.Now().Add(-2 * keepFinished)
	if err := os.Chtimes(read, old, old); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.t.Fatal(err)
	}
}

// sweepR is r's own sweep, while it lives.
func (s *matrixScene) sweepR() {
	s.t.Helper()
	sweepAsLiveRun(s.t, s.dir, "web", s.r.epoch())
}
