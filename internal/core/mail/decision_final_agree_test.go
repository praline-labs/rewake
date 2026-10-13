package mail

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// A decision's final read and the barrier come to the same answer on the
// same evidence (finalEvidence): the decision is refused as proven exactly
// where the barrier settles the report, and made exactly where it cannot. For
// every evidence R's mailbox may hold, with R's run live and ended, under an
// occurrence a plan found at R's mark and one only an effect met at R's
// letter's place, one room asks the decision and a twin runs the barrier.
func TestDecisionFinalReadAgreesWithTheBarrier(t *testing.T) {
	evidence := map[string]func(s *matrixScene, r Message){
		"the letter and its published mark": func(s *matrixScene, r Message) { s.landDirect(r); s.writeMark(r, oncePublished) },
		"the letter, its mark damaged":      func(s *matrixScene, r Message) { s.landDirect(r); s.writeMark(r, "damaged") },
		"the letter, no mark":               func(s *matrixScene, r Message) { s.landDirect(r); s.dropMark(r) },
		"a published mark":                  func(s *matrixScene, r Message) { s.writeMark(r, oncePublished) },
		"an intent":                         func(s *matrixScene, r Message) { s.writeMark(r, onceIntent) },
		"no mark":                           func(s *matrixScene, r Message) { s.dropMark(r) },
		"a damaged mark":                    func(s *matrixScene, r Message) { s.writeMark(r, "damaged") },
		"an intent, the letter's place faulty": func(s *matrixScene, r Message) {
			s.writeMark(r, onceIntent)
			s.faultLetter(r.ID)
		},
	}
	occurrences := map[string]func(t *testing.T) (*matrixScene, Message){
		"found at the mark":                                               newContractScene,
		"met only by the effect at its place":                             newEffectScene,
		"met only by the effect at its place, the letter there then gone": newEffectSceneLetterGone,
	}
	for occurrence, build := range occurrences {
		for _, ended := range []bool{false, true} {
			for name, returns := range evidence {
				label := occurrence + ", the run live, " + name
				if ended {
					label = occurrence + ", the run ended, " + name
				}
				t.Run(label, func(t *testing.T) {
					t.Parallel()
					room := func() (*matrixScene, Message) {
						s, r := build(t)
						if ended {
							s.endR()
						}
						returns(s, r)
						return s, r
					}
					asked, r := room()
					answer, err := asked.decide(r, ChoiceNotArrived, false)
					proven := outcomeOf(err) == DecisionProven
					if !proven && err != nil {
						t.Fatalf("the decision answered %v", err)
					}
					if proven == (answer.Decision.ID != "") {
						t.Fatalf("the decision answered %+v, %v", answer.Decision, err)
					}
					twin, r2 := room()
					published, moot := twin.journalRecords(r2, twin.barrier)
					if settled := published || moot; settled != proven {
						t.Errorf("the barrier settled R %v (published %v, moot %v); the decision was refused as proven %v (%v)", settled, published, moot, proven, err)
					}
				})
			}
		}
	}
}

// newEffectScene is a room whose report R has one occurrence, which only the
// effect met at R's letter's place, cleared since; R's mark is an intent.
func newEffectScene(t *testing.T) (*matrixScene, Message) {
	t.Helper()
	s := newMatrixScene(t, false)
	r := s.report
	s.writeMark(r, onceIntent)
	s.fault(false, true, r)
	if err := s.barrier(); err == nil {
		t.Fatal("the barrier went past R's unknown outcome")
	}
	s.fault(false, false, r)
	open := stopsOnRecord(t, s.dir, "api")
	if len(open) != 1 || open[0].record.Kind != causeEffect {
		t.Fatalf("the occurrences open: %v", open)
	}
	return s, r
}

// newEffectSceneLetterGone is newEffectScene with R's letter in its place
// when the effect met it, and gone since: a path the occurrence names was
// removed while stopped, which is no evidence, so the barrier holds its
// effects until R's letter is back.
func newEffectSceneLetterGone(t *testing.T) (*matrixScene, Message) {
	t.Helper()
	s := newMatrixScene(t, false)
	r := s.report
	s.writeMark(r, onceIntent)
	s.landDirect(r)
	letter := filepath.Join(state.InboxPath(s.dir, "web"), r.ID+".json")
	s.access.mu.Lock()
	s.access.stat = func(path string, plan bool) (fs.FileInfo, error) {
		if !plan && path == letter {
			return notRegular{filepath.Base(path)}, nil
		}
		return nil, nil
	}
	s.access.mu.Unlock()
	if err := s.barrier(); err == nil {
		t.Fatal("the barrier went past R's unknown outcome")
	}
	s.fault(false, false, r)
	open := stopsOnRecord(t, s.dir, "api")
	if len(open) != 1 || open[0].record.Kind != causeEffect || !slices.Equal(open[0].record.Present, []string{relative(s.dir, letter)}) {
		t.Fatalf("the occurrences open: %+v", open)
	}
	if err := os.Remove(letter); err != nil {
		t.Fatal(err)
	}
	return s, r
}

// dropMark removes R's mark, as a sweep of its ended run's marks would.
func (s *matrixScene) dropMark(r Message) {
	if err := os.Remove(s.markPath(r)); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.t.Fatal(err)
	}
}

// journalRecords runs pass and answers what J's saves recorded of R, read
// before each step, since its done form keeps none of it.
func (s *matrixScene) journalRecords(r Message, pass func() error) (published, moot bool) {
	open := filepath.Join(JournalPath(s.dir, "api"), "end")
	s.access.mu.Lock()
	s.access.beforeOp = func(string, string) error {
		if saved, err := readJournalFile(open); err == nil {
			published = published || slices.Contains(saved.Published, r.ID)
			moot = moot || slices.Contains(saved.Moot, r.ID)
		}
		return nil
	}
	s.access.mu.Unlock()
	_ = pass()
	s.access.mu.Lock()
	s.access.beforeOp = nil
	s.access.mu.Unlock()
	return published, moot
}
