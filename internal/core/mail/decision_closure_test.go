package mail

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// A decided report's obligation is closed once, by J's tail, and the record
// of it outlives J (docs/v2/stage3-decision-recovery.md, the closure): a
// tail replayed after a crash finds its closure written and clears no wait
// that came after; J done and swept, the closure still says the wait was
// closed by D; an interim decided closes no wait, so the work it reported on
// stays owed; and the question R answers is said to be closed by D only once
// the tail ran, never answered, while before it the decision leaves it owed.
func TestDecisionClosureKeepsProvenance(t *testing.T) {
	t.Run("an old tail retried", func(t *testing.T) {
		t.Parallel()
		s, r := newContractScene(t)
		journal := filepath.Join(JournalPath(s.dir, "api"), "end")
		done := journal + doneSuffix
		closure := DecisionPath(s.dir, "api", r.ID) + closedSuffix
		// The crash comes between the closure and J's done form: J then
		// reads as unfinished, and the next barrier runs its tail again.
		failed := false
		s.access.mu.Lock()
		s.access.beforeOp = func(op, path string) error {
			if op == "write" && path == journal && !failed && fileExists(closure) {
				failed = true
				return &fs.PathError{Op: op, Path: path, Err: syscall.EIO}
			}
			return nil
		}
		s.access.mu.Unlock()
		_, _ = s.decide(r, ChoiceArrived, false)
		if !failed {
			t.Fatal("J never went to its done rewrite")
		}
		if j, err := readJournalFile(journal); err != nil || j.Done {
			t.Fatalf("J after the crash: done %v, %v", j.Done, err)
		}
		first, err := os.ReadFile(closure)
		if err != nil {
			t.Fatalf("the tail before the crash wrote no closure: %v", err)
		}
		later := NewID()
		if err := markAwaiting(s.dir, "api", s.epoch, "web", r.ToEpoch, later); err != nil {
			t.Fatal(err)
		}
		if err := s.barrier(); err != nil {
			t.Fatalf("the replay: %v", err)
		}
		if again, err := os.ReadFile(closure); err != nil || !bytes.Equal(again, first) {
			t.Errorf("the replay changed the closure: %s, %v", again, err)
		}
		waits, err := ReadWaiters(s.dir, "api", s.epoch)
		if err != nil || len(waits) != 1 || waits[0].Messages[0] != later {
			t.Errorf("the replay cleared a later wait: %+v, %v", waits, err)
		}
		if !fileExists(done) {
			t.Error("J is not done after its replay")
		}
	})

	t.Run("J gone and still closed by D", func(t *testing.T) {
		t.Parallel()
		s, r := newContractScene(t)
		answer, err := s.decide(r, ChoiceArrived, false)
		if err != nil {
			t.Fatal(err)
		}
		sweepTurnRecords(s.dir, "api", "a-later-run", time.Now().Add(time.Hour))
		if fileExists(filepath.Join(JournalPath(s.dir, "api"), "end"+doneSuffix)) {
			t.Fatal("the sweep kept J")
		}
		mark, ok := DecisionOn(s.dir, "api", r.InReplyTo[0], "web", r.ToEpoch)
		if !ok || !mark.Closed || mark.Decision != answer.Decision.ID {
			t.Errorf("with J gone, the question reads %+v, %v", mark, ok)
		}
		s.views = nil
		if o := s.outcome(t, r); o.closure == nil || o.closure.Decision != answer.Decision.ID || o.closure.Journal != "end" || o.waits != 0 {
			t.Errorf("with J gone: closure %+v, waits %d", o.closure, o.waits)
		}
	})

	t.Run("an interim closes no task", func(t *testing.T) {
		t.Parallel()
		s := newMatrixScene(t, false)
		r := s.interimReport()
		s.writeMark(r, "damaged")
		if err := s.barrier(); err == nil {
			t.Fatal("the barrier went past R's unknown outcome")
		}
		if _, err := s.decide(r, ChoiceNotArrived, false); err != nil {
			t.Fatal(err)
		}
		closure := s.outcome(t, r).closure
		if closure == nil || len(closure.Waits) != 0 {
			t.Errorf("the interim's closure %+v closes waits", closure)
		}
		if waits, err := ReadWaiters(s.dir, "api", s.epoch); err != nil || len(waits) != 1 {
			t.Errorf("the task's wait after an interim decided: %+v, %v", waits, err)
		}
		if mark, ok := DecisionOn(s.dir, "api", r.InReplyTo[0], "web", r.ToEpoch); ok && mark.Closed {
			t.Errorf("the question reads closed by an interim: %+v", mark)
		}
		resend, err := s.outcome(t, r).decision.resendJournal()
		if err != nil || resend.Reports[0].Kind != Interim || resend.Interim != nil || len(resend.Clear) != 0 {
			t.Errorf("the interim sent again as %+v, %v", resend, err)
		}
		if n := s.access.written(resend.Reports[0].ID); n != 1 {
			t.Errorf("the interim's copy was written %d times", n)
		}
	})

	t.Run("a blocked question's truthful end", func(t *testing.T) {
		t.Parallel()
		s, r := newContractScene(t)
		question := r.InReplyTo[0]
		unrelated := filepath.Join(JournalPath(s.dir, "api"), "zzz")
		writeRaw(t, unrelated, "{")
		answer, err := s.decide(r, ChoiceArrived, false)
		if err != nil {
			t.Fatal(err)
		}
		mark, ok := DecisionOn(s.dir, "api", question, "web", r.ToEpoch)
		if !ok || mark.Closed || mark.Decision != answer.Decision.ID {
			t.Errorf("before the tail, the question reads %+v, %v", mark, ok)
		}
		if waits, err := ReadWaiters(s.dir, "api", s.epoch); err != nil || len(waits) != 1 {
			t.Errorf("before the tail, the wait: %+v, %v", waits, err)
		}
		if err := WriteJournal(s.dir, "api", "zzz", TurnJournal{Epoch: s.epoch}); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			_ = s.barrier()
		}
		mark, ok = DecisionOn(s.dir, "api", question, "web", r.ToEpoch)
		if !ok || !mark.Closed || mark.Decision != answer.Decision.ID {
			t.Errorf("after the tail, the question reads %+v, %v", mark, ok)
		}
		if n := s.access.written(r.ID); n != 0 {
			t.Errorf("R was written %d times", n)
		}
		if entries, _ := os.ReadDir(filepath.Join(state.InboxPath(s.dir, "web"), "received")); len(entries) != 0 {
			t.Errorf("the question received an answer: %v", entries)
		}
	})
}

// interimReport has api's journal end as an interim end instead, carrying R
// as an interim report on the task web asked, the task's wait standing.
func (s *matrixScene) interimReport() Message {
	s.t.Helper()
	waiters, err := ReadWaiters(s.dir, "api", s.epoch)
	if err != nil || len(waiters) != 1 {
		s.t.Fatalf("the wait: %v %v", waiters, err)
	}
	line := "still on it"
	report := Message{ID: ReportID("api", s.epoch, waiters[0]), From: "api", FromEpoch: s.epoch, To: "web", ToEpoch: s.r.epoch(), Kind: Interim, Text: line, InReplyTo: waiters[0].Messages, CreatedAt: time.Now()}
	if err := WriteJournal(s.dir, "api", "end", TurnJournal{Epoch: s.epoch, Reports: []Message{report}, Interim: &line}); err != nil {
		s.t.Fatal(err)
	}
	return report
}
