package mail

import (
	"bytes"
	"os"
	"strings"
	"testing"

	dm "github.com/praline-labs/rewake/internal/core/mail/decisionmodel"
)

// playDecision plays one world of the decision partition
// (docs/v2/stage3-decision-manifest.md#the-schedules-held-fixed): R's
// history and the fault that makes its evidence unknown, the barrier that
// stops on it while r lives, r's end before D with what r did while it
// lived, D or its refusal, then — each followed by the sender's barrier
// while the sender lives — a second command, R's evidence returning, what r
// does after D, r's end, a new run of its name, and the recovery. It answers
// the durable steps the product took, and checks what the world must end
// with against the model.
func playDecision(t *testing.T, w dm.World, cut matrixCut) []string {
	v := w.Value
	verified := v("decider") == dm.ByRecipient
	s := newMatrixScene(t, verified)
	s.access.cut, s.access.flush = cut.at, cut.flush
	r := s.report
	landed, at, later := v("truth") == dm.Landed, v("at-decision"), v("later")
	choice := matrixChoice(v("choice"))
	endsBefore, endsAfter := v("r-at-decision") == dm.REndsBeforeD, v("r-after-decision") == dm.REndsAfterD
	activity, sender := v("activity"), v("sender")

	mark := onceIntent
	if at == dm.MarkUnreadable {
		mark = "damaged"
	}
	if landed {
		s.land(r, mark)
	} else {
		s.writeMark(r, mark)
	}
	s.initialLanding = s.access.written(r.ID)
	s.fault(at == dm.MailboxUnsearchable, at == dm.EffectMet, r)
	if err := s.barrier(); err == nil {
		t.Fatal("the barrier went past R's unknown outcome")
	}
	if endsBefore {
		s.whileAlive(activity, r)
		s.endR()
	}
	if later == dm.ReturnsBeforeD {
		s.repair(at, landed, r)
	}

	// Evidence that returned settles R only while r lives (S3): with r
	// ended, a decision is still the only way out.
	decided := later != dm.ReturnsBeforeD || endsBefore
	var answer DecideAnswer
	err := s.product(func() error {
		var err error
		answer, err = s.decide(r, choice, verified)
		return err
	})
	s.afterD(answer.Decision.ID != "" || err == nil)
	if !decided {
		if outcomeOf(err) != DecisionProven {
			t.Fatalf("with R's evidence back, decide answered %+v, %v", answer, err)
		}
	} else if err != nil {
		t.Fatalf("the decision was not made: %v", err)
	}
	var first []byte
	if decided {
		first = s.decisionBytes(t, r)
	}
	alive := sender == dm.SenderAlive
	if alive || sender == dm.SenderEndsAfter {
		s.senderBarrier()
	}
	switch v("second") {
	case dm.SameWords:
		var again DecideAnswer
		err := s.product(func() error {
			var err error
			again, err = s.decide(r, choice, verified)
			return err
		})
		s.afterD(true)
		if err != nil || !again.Before || again.Decision.ID != answer.Decision.ID {
			t.Errorf("the same words again answered %+v, %v", again, err)
		}
	case dm.OppositeWords:
		_, err := s.decide(r, matrixOpposite(choice), verified)
		if outcomeOf(err) != DecisionOpposite {
			t.Errorf("the opposite words answered %v", err)
		}
	}
	if later == dm.ReturnsWritten || later == dm.ReturnsAbsent {
		s.repair(at, landed, r)
	}
	if alive {
		s.senderBarrier()
	}
	if !endsBefore {
		s.whileAlive(activity, r)
	}
	if endsAfter {
		s.endR()
	}
	if activity == dm.NewRunDropsMark {
		if later == dm.RetiredProof {
			s.fault(false, false, r)
		}
		s.newRunSweeps()
	}
	if alive {
		s.senderBarrier()
	}
	s.recover(alive, decided, r, choice, verified)

	o := s.outcome(t, r)
	switch {
	case !decided:
		s.checkUndecided(t, o, r)
	default:
		s.checkDecided(t, o, r, choice, verified, first)
	}
	return s.access.steps()
}

// whileAlive is what r does in its mailbox while it lives: it reads R, or
// reads it and sweeps; a letter that never landed is not there to read.
func (s *matrixScene) whileAlive(activity string, r Message) {
	switch activity {
	case dm.RReads:
		s.readR(r)
	case dm.SweepPromotes:
		s.readR(r)
		s.sweepR()
	}
}

// repair has R's evidence return as the world's truth: the mark read again,
// published or intent, and the faults on the mailbox gone.
func (s *matrixScene) repair(at string, landed bool, r Message) {
	if at == dm.MarkUnreadable {
		mark := onceIntent
		if landed {
			mark = oncePublished
		}
		s.writeMark(r, mark)
		return
	}
	s.fault(false, false, r)
}

// senderBarrier is the sender's barrier, armed.
func (s *matrixScene) senderBarrier() {
	_ = s.product(s.barrier)
	s.afterD(true)
}

// recover is what follows the schedule: the sender's barriers while it
// lives; with it ended, the same words of the deciding command, which is all
// that runs a barrier then.
func (s *matrixScene) recover(alive, decided bool, r Message, choice string, verified bool) {
	if alive || !decided {
		for range 2 {
			s.senderBarrier()
		}
		return
	}
	_ = s.product(func() error {
		_, err := s.decide(r, choice, verified)
		return err
	})
	s.afterD(true)
}

func (s *matrixScene) decisionBytes(t *testing.T, r Message) []byte {
	t.Helper()
	raw, err := os.ReadFile(DecisionPath(s.dir, "api", r.ID))
	if err != nil {
		t.Fatalf("no decision on R: %v", err)
	}
	return raw
}

func matrixChoice(value string) string {
	if value == dm.Arrived {
		return ChoiceArrived
	}
	return ChoiceNotArrived
}

func matrixOpposite(choice string) string {
	if choice == ChoiceArrived {
		return ChoiceNotArrived
	}
	return ChoiceArrived
}

// checkUndecided is a world whose evidence returned while r lived: no
// decision, R published once — written before or now — and the journal
// finished.
func (s *matrixScene) checkUndecided(t *testing.T, o matrixOutcome, r Message) {
	t.Helper()
	if o.decisions != 0 {
		t.Errorf("%d decisions on R after its evidence returned", o.decisions)
	}
	if n := s.access.written(r.ID); n != 1 {
		t.Errorf("R was written %d times", n)
	}
	if o.waits != 0 || len(o.open) != 0 {
		t.Errorf("waits %d, occurrences open %v", o.waits, o.open)
	}
}

// checkDecided holds a decided world to the model: the copies its truth and
// choices predict, each written at most once; the record whole, immutable
// and saying nothing delivered; the notes the model owes; and the waits
// cleared once nothing in the mailbox stays unknown.
func (s *matrixScene) checkDecided(t *testing.T, o matrixOutcome, r Message, choice string, verified bool, first []byte) {
	t.Helper()
	if o.decisions != 1 || o.decision == nil {
		t.Fatalf("%d decisions on R", o.decisions)
	}
	d := *o.decision
	if !bytes.Equal(first, s.decisionBytes(t, r)) {
		t.Error("the decision's record changed after it was written")
	}
	wantBy := DecidedBy{Kind: ByPerson}
	if verified {
		wantBy = DecidedBy{Kind: ByRecipient, Name: "web", Run: r.ToEpoch, Verified: true}
	}
	if d.Choice != choice || d.By != wantBy || d.Delivery != deliveryUnproven || d.Subject.Report != r.ID {
		t.Errorf("the decision says %s by %+v, %s, about %s", d.Choice, d.By, d.Delivery, d.Subject.Report)
	}
	chain := []dm.Copy{{Landed: s.initialLanding > 0, Decided: matrixModelChoice(choice)}}
	if want := s.initialLanding; s.access.written(r.ID) != want {
		t.Errorf("R was written %d times; it landed %d times before D", s.access.written(r.ID), want)
	}
	var resend matrixFate
	if choice == ChoiceNotArrived {
		resend = s.resendOutcome()
		copied := o.resent
		if copied == "" {
			t.Fatal("no resend for \"did not arrive\"")
		}
		n := s.access.written(copied)
		if n > 1 || (n == 1) != resend.landed {
			t.Errorf("R' was written %d times; the world lands it %v", n, resend.landed)
		}
		chain = append(chain, dm.Copy{Landed: resend.landed})
	}
	want := dm.Expect(chain)
	landings := s.access.written(r.ID)
	if o.resent != "" {
		landings += s.access.written(o.resent)
	}
	if landings != want.Landings {
		t.Errorf("%d landings; the model says %+v", landings, want)
	}

	cleared := dm.WaitsCleared(true, !resend.held)
	if (o.waits == 0) != cleared {
		t.Errorf("R's waits: %d left; the model clears them: %v", o.waits, cleared)
	}
	if cleared && (o.closure == nil || o.closure.Decision != d.ID || o.closure.Report != r.ID) {
		t.Errorf("R's step is not recorded closed by D: %+v", o.closure)
	}
	var wantOpen []string
	if resend.held {
		wantOpen = []string{o.resent}
	}
	if strings.Join(o.open, ",") != strings.Join(wantOpen, ",") {
		t.Errorf("occurrences open about %v; the world leaves %v", o.open, wantOpen)
	}
	s.checkNotes(t, d, verified, s.noteFate())
}

func matrixModelChoice(choice string) string {
	if choice == ChoiceArrived {
		return dm.Arrived
	}
	return dm.NotArrived
}
