package mail

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// A report an open occurrence is about stays where it is when its run ends,
// until evidence returns (docs/mailbox-records.md): the section that finds
// the run ended reads the evidence without admitting it. The letter and its
// published mark, or the mark alone, say it landed, and it is recorded
// published with nothing written; an intent with no letter says it never
// did, and it is moot; a mark still unreadable, or none, keeps it where it
// is. While its run lives, the evidence is read as before.
func TestAHeldReportOfAnEndedRunIsSettledByItsEvidence(t *testing.T) {
	cases := map[string]struct {
		alive   bool
		returns func(s *matrixScene, r Message)
		// disposition is what J records of R; empty when R stays held.
		disposition string
		written     int
	}{
		"the letter and its published mark": {false, func(s *matrixScene, r Message) {
			s.landDirect(r)
			s.writeMark(r, oncePublished)
		}, DispositionPublished, 1},
		"the published mark, the letter read and swept": {false, func(s *matrixScene, r Message) {
			s.writeMark(r, oncePublished)
		}, DispositionPublished, 0},
		"an intent with no letter": {false, func(s *matrixScene, r Message) {
			s.writeMark(r, onceIntent)
		}, DispositionMoot, 0},
		"the mark still unreadable": {false, func(*matrixScene, Message) {}, "", 0},
		"no mark at all": {false, func(s *matrixScene, r Message) {
			if err := os.Remove(s.markPath(r)); err != nil {
				t.Fatal(err)
			}
		}, "", 0},
		"an intent with no letter, the run alive": {true, func(s *matrixScene, r Message) {
			s.writeMark(r, onceIntent)
		}, DispositionPublished, 1},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := newMatrixScene(t, false)
			r := s.report
			s.writeMark(r, "damaged")
			if err := s.barrier(); err == nil {
				t.Fatal("the barrier went past R's unknown outcome")
			}
			if !c.alive {
				s.endR()
				if err := s.barrier(); err == nil {
					t.Fatal("the run's end settled R with nothing returned")
				}
			}
			c.returns(s, r)
			// What J's saves record of R, read before each step: its done form
			// keeps none of it.
			var journal TurnJournal
			var readErr error = os.ErrNotExist
			open := filepath.Join(JournalPath(s.dir, "api"), "end")
			s.access.mu.Lock()
			s.access.beforeOp = func(string, string) error {
				if saved, err := readJournalFile(open); err == nil && !saved.Done && len(saved.Published)+len(saved.Moot) > 0 {
					journal, readErr = saved, nil
				}
				return nil
			}
			s.access.mu.Unlock()
			err := s.barrier()
			if n := s.access.written(r.ID); n != c.written {
				t.Errorf("R was written %d times, want %d", n, c.written)
			}
			if c.disposition == "" {
				if err == nil || readErr == nil {
					t.Fatalf("R settled on no evidence: %v, the journal done: %v", err, readErr == nil)
				}
				if open := stopsOnRecord(t, s.dir, "api"); len(open) != 1 || open[0].reportOf() != r.ID {
					t.Errorf("the occurrences open: %v", open)
				}
				return
			}
			if err != nil || readErr != nil {
				t.Fatalf("the barrier once the evidence returned: %v; the journal: %v", err, readErr)
			}
			got := map[bool]string{true: DispositionPublished, false: DispositionMoot}[slices.Contains(journal.Published, r.ID)]
			if got != c.disposition || slices.Contains(journal.Moot, r.ID) == slices.Contains(journal.Published, r.ID) {
				t.Errorf("J recorded R published %v, moot %v; want %s", journal.Published, journal.Moot, c.disposition)
			}
			if open := stopsOnRecord(t, s.dir, "api"); len(open) != 0 {
				t.Errorf("occurrences still open: %v", open)
			}
		})
	}
}

// endedEvidence reads only what proves a landing or its absence: the letter
// in any stage or a published mark, an intent with no letter; no mark, a mark
// that does not read or a letter's place that cannot be searched is no
// evidence, whatever else is there.
func TestEndedEvidenceReadsOnlyProof(t *testing.T) {
	cases := map[string]struct {
		set       func(s *matrixScene, r Message)
		published bool
		none      bool
	}{
		"the letter, no mark":         {func(s *matrixScene, r Message) { s.landDirect(r) }, true, false},
		"the letter read, no mark":    {func(s *matrixScene, r Message) { s.landDirect(r); s.readR(r) }, true, false},
		"a published mark":            {func(s *matrixScene, r Message) { s.writeMark(r, oncePublished) }, true, false},
		"an intent, no letter":        {func(s *matrixScene, r Message) { s.writeMark(r, onceIntent) }, false, false},
		"nothing":                     {func(*matrixScene, Message) {}, false, true},
		"a mark that does not read":   {func(s *matrixScene, r Message) { s.writeMark(r, "damaged") }, false, true},
		"an intent, its place faulty": {func(s *matrixScene, r Message) { s.writeMark(r, onceIntent); s.faultLetter(r.ID) }, false, true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := newMatrixScene(t, false)
			r := s.report
			c.set(s, r)
			published, err := live(s.dir).endedEvidence(r)
			if (err != nil) != c.none || err != nil && !errors.Is(err, errNoEvidence) || published != c.published {
				t.Errorf("endedEvidence answered %v, %v; want published %v, no evidence %v", published, err, c.published, c.none)
			}
		})
	}
}
