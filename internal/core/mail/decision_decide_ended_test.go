package mail

import (
	"os"
	"testing"
)

// With R's run ended, decide reads R's evidence as the barrier reads a held
// report's (endedEvidence): the letter or a published mark proves it
// published and an intent with no letter proves it never was, so nothing is
// decided and nothing written; no mark at all, or one that does not read,
// proves nothing, and the decision is made.
func TestDecideOnAHeldReportOfAnEndedRunReadsItsEvidence(t *testing.T) {
	cases := map[string]struct {
		returns func(s *matrixScene, r Message)
		proven  bool
		// letterPlace has R's unknown one only its effect met — its letter's
		// place holds no letter — the mark an intent: an occurrence whose paths
		// the decision does not read.
		letterPlace bool
	}{
		"the letter and its published mark": {func(s *matrixScene, r Message) { s.landDirect(r); s.writeMark(r, oncePublished) }, true, false},
		"a published mark":                  {func(s *matrixScene, r Message) { s.writeMark(r, oncePublished) }, true, false},
		"an intent with no letter":          {func(s *matrixScene, r Message) { s.writeMark(r, onceIntent) }, true, false},
		"the mark still unreadable":         {func(*matrixScene, Message) {}, false, false},
		"no mark at all": {func(s *matrixScene, r Message) {
			if err := os.Remove(s.markPath(r)); err != nil {
				t.Fatal(err)
			}
		}, false, false},
		"no mark, the place only the effect met cleared": {func(s *matrixScene, r Message) {
			if err := os.Remove(s.markPath(r)); err != nil {
				t.Fatal(err)
			}
			s.fault(false, false, r)
		}, false, true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var s *matrixScene
			var r Message
			if c.letterPlace {
				s = newMatrixScene(t, false)
				r = s.report
				s.writeMark(r, onceIntent)
				s.fault(false, true, r)
				if err := s.barrier(); err == nil {
					t.Fatal("the barrier went past R's unknown outcome")
				}
			} else {
				s, r = newContractScene(t)
			}
			s.endR()
			c.returns(s, r)
			before := s.access.written(r.ID)
			answer, err := s.decide(r, ChoiceNotArrived, false)
			if c.proven {
				if outcomeOf(err) != DecisionProven || answer.Decision.ID != "" || fileExists(DecisionPath(s.dir, "api", r.ID)) {
					t.Fatalf("decide on proof answered %+v, %v", answer, err)
				}
			} else if err != nil || !fileExists(DecisionPath(s.dir, "api", r.ID)) {
				t.Fatalf("decide on no proof answered %+v, %v", answer, err)
			}
			if n := s.access.written(r.ID); n != before {
				t.Errorf("R was written %d times, %d before", n, before)
			}
		})
	}
}
