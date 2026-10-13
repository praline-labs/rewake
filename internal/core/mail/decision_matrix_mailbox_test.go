package mail

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	dm "github.com/praline-labs/rewake/internal/core/mail/decisionmodel"
)

// playMailbox plays one world of the mailbox partition: R, never landed, its
// mark unreadable, beside what the world puts in api's mailbox — a second
// report of J, to ops, unknown the same way; a journal that does not read,
// named before J or after it; or R' turning unknown at its mark as it first
// runs. r, verified and alive, decides R; the person decides the second
// report when the world says so; the rest is settled as its value names, or
// left; and the sender's barriers follow.
func playMailbox(t *testing.T, w dm.World, cut matrixCut) []string {
	v := w.Value
	rest, second, choice := v("rest"), v("second-report"), matrixChoice(v("choice"))
	s := newMatrixScene(t, true)
	s.access.cut, s.access.flush = cut.at, cut.flush
	r := s.report
	var r2 Message
	if second != dm.None {
		r2 = s.secondReport()
	}
	s.writeMark(r, "damaged")
	if err := s.barrier(); err == nil {
		t.Fatal("the barrier went past R's unknown outcome")
	}
	unrelated := map[string]string{
		dm.UnrelatedBeforeSettled: "aaa", dm.UnrelatedBeforeLeft: "aaa",
		dm.UnrelatedAfterSettled: "zzz", dm.UnrelatedAfterLeft: "zzz",
	}[rest]
	// The journal turns unreadable once every report the world decides is on
	// record: a barrier records the first unknown it meets, an unreadable
	// journal is met before any step, and J's second report only after R is
	// decided.
	corrupt := func() {
		if unrelated == "" {
			return
		}
		writeRaw(t, filepath.Join(JournalPath(s.dir, "api"), unrelated), "{")
		if err := s.barrier(); err == nil {
			t.Fatal("the barrier went past a journal that does not read")
		}
	}
	if second == dm.None {
		corrupt()
	}
	child := strings.HasPrefix(rest, "child-")
	if child {
		s.childTurnsUnknown(r)
	}

	if err := s.product(func() error {
		_, err := s.decide(r, choice, true)
		return err
	}); err != nil {
		t.Fatalf("the decision was not made: %v", err)
	}
	if second != dm.None {
		corrupt()
	}
	if second == dm.SecondDecidedArrived || second == dm.SecondDecidedNotArrived {
		if err := s.product(func() error {
			_, err := s.decide(r2, matrixChoice(map[string]string{
				dm.SecondDecidedArrived: dm.Arrived, dm.SecondDecidedNotArrived: dm.NotArrived,
			}[second]), false)
			return err
		}); err != nil {
			t.Fatalf("the decision on the second report was not made: %v", err)
		}
	}
	_ = s.product(s.barrier)

	copied := s.resentOf(r)
	switch rest {
	case dm.ChildUnknownEvidence:
		s.writeMark(Message{ID: copied, To: "web", ToEpoch: r.ToEpoch}, onceIntent)
	case dm.ChildUnknownDecision:
		if err := s.product(func() error {
			_, err := s.decide(Message{ID: copied, To: "web", ToEpoch: r.ToEpoch}, ChoiceArrived, true)
			return err
		}); err != nil {
			t.Fatalf("the decision on R' was not made: %v", err)
		}
	case dm.UnrelatedBeforeSettled, dm.UnrelatedAfterSettled:
		if err := WriteJournal(s.dir, "api", unrelated, TurnJournal{Epoch: s.epoch}); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		_ = s.product(s.barrier)
	}

	s.checkMailbox(t, w, r, r2, copied, unrelated)
	return s.access.steps()
}

// secondReport puts a second report of J beside R, to ops, a run of its own,
// its mark unreadable as R's.
func (s *matrixScene) secondReport() Message {
	s.t.Helper()
	ops := startRoleRun(s.t, s.dir, "ops", "")
	r2 := Message{ID: NewID(), From: "api", FromEpoch: s.epoch, To: "ops", ToEpoch: ops.session.Epoch(), Kind: Finished, Text: "the other answer", CreatedAt: time.Now()}
	journal, err := readJournalFile(filepath.Join(JournalPath(s.dir, "api"), "end"))
	if err != nil {
		s.t.Fatal(err)
	}
	journal.Reports = append(journal.Reports, r2)
	if err := WriteJournal(s.dir, "api", "end", journal); err != nil {
		s.t.Fatal(err)
	}
	s.writeMark(r2, "damaged")
	return r2
}

// childTurnsUnknown has R'’s mark unreadable as J' is installed, before R'
// is first tried.
func (s *matrixScene) childTurnsUnknown(r Message) {
	sep := string(filepath.Separator)
	done := false
	s.access.mu.Lock()
	defer s.access.mu.Unlock()
	s.access.beforeOp = func(op, path string) error {
		if done || op != "publish" || !strings.Contains(path, sep+"journal"+sep+"decided-") {
			return nil
		}
		done = true
		s.writeMark(Message{ID: s.resentOf(r), To: "web", ToEpoch: r.ToEpoch}, "damaged")
		return nil
	}
}

// checkMailbox holds a mailbox world to the model: nothing publishes and R's
// waits stand while an unknown stands anywhere in the mailbox, and once none
// does the tail runs once; each copy lands as its truth and the choices say.
// A barrier records the first unknown it meets — a journal that does not
// read before any step, then the steps by journal name, J' before J — so the
// occurrences left open are some of those the world leaves, never one more.
func (s *matrixScene) checkMailbox(t *testing.T, w dm.World, r, r2 Message, copied, unrelated string) {
	t.Helper()
	v := w.Value
	rest, second, choice := v("rest"), v("second-report"), v("choice")
	undecided := second == dm.SecondUndecided
	unrelatedLeft := rest == dm.UnrelatedBeforeLeft || rest == dm.UnrelatedAfterLeft
	childLeft := rest == dm.ChildUnknownLeft
	admitted := !undecided && !unrelatedLeft && !childLeft
	// A copy is written only in an admitted barrier after its decision; R'
	// meets its own unknown first when the world gives it one.
	landed := choice == dm.NotArrived && admitted && rest != dm.ChildUnknownDecision

	o := s.outcome(t, r)
	if o.decisions != 1 || o.decision.Choice != matrixChoice(choice) {
		t.Fatalf("the decision on R: %d, %+v", o.decisions, o.decision)
	}
	if n := s.access.written(r.ID); n != 0 {
		t.Errorf("R was written %d times", n)
	}
	s.checkNotes(t, *o.decision, true, "")
	chain := []dm.Copy{{Decided: choice}}
	if choice == dm.NotArrived {
		if n := s.access.written(copied); n > 1 || (n == 1) != landed {
			t.Errorf("R' was written %d times; the world lands it %v", n, landed)
		}
		next := dm.Copy{Landed: landed}
		if rest == dm.ChildUnknownDecision {
			next.Decided = dm.Arrived
			if inner := s.outcome(t, Message{ID: copied}); inner.decisions != 1 {
				t.Errorf("%d decisions on R'", inner.decisions)
			} else {
				s.checkNotes(t, *inner.decision, true, "")
			}
		}
		chain = append(chain, next)
	}
	if want, n := dm.Expect(chain), s.access.written(copied); n != want.Landings {
		t.Errorf("%d landings; the model says %+v", n, want)
	}

	allowed := map[string]bool{}
	if unrelatedLeft {
		allowed[unrelated] = true
	}
	if undecided {
		allowed[r2.ID] = true
	}
	if childLeft {
		allowed[copied] = true
	}
	for _, id := range o.open {
		if !allowed[id] {
			t.Errorf("an occurrence open about %s; the world leaves %v", id, allowed)
		}
	}
	if (len(o.open) == 0) != admitted {
		t.Errorf("occurrences open about %v; the world admits the mailbox: %v", o.open, admitted)
	}
	cleared := dm.WaitsCleared(!undecided, admitted)
	if (o.waits == 0) != cleared {
		t.Errorf("R's waits: %d left; the model clears them: %v", o.waits, cleared)
	}
	if cleared != (o.closure != nil) {
		t.Errorf("R's step closed by D: %+v; the tail ran: %v", o.closure, cleared)
	}
	if second == dm.None {
		return
	}
	if n := s.access.written(r2.ID); n != 0 {
		t.Errorf("the second report was written %d times", n)
	}
	o2 := s.outcome(t, r2)
	if undecided {
		if o2.decisions != 0 {
			t.Errorf("%d decisions on the undecided second report", o2.decisions)
		}
		return
	}
	if o2.decisions != 1 {
		t.Fatalf("%d decisions on the second report", o2.decisions)
	}
	s.checkNotes(t, *o2.decision, false, ToldWritten)
	if second == dm.SecondDecidedNotArrived {
		if n := s.access.written(o2.resent); n > 1 || (n == 1) != admitted {
			t.Errorf("the second report's copy was written %d times; the world lands it %v", n, admitted)
		}
	}
}
