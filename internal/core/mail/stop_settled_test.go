package mail

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// A resolution of an occurrence about a report is durable before the journal
// records what its evidence settled (stop_settled.go): a pass that only looks
// at the stop resolves it and runs no journal, and a process may stop between
// the resolution and the journal's save. Neither may cost the ended run's
// report its disposition: the next barrier settles it by the same evidence,
// never as an ordinary report of an ended run, which would be moot.
func TestAResolvedOccurrenceKeepsItsReportsDisposition(t *testing.T) {
	cases := map[string]struct {
		returns     func(s *matrixScene, r Message)
		disposition string
	}{
		"the letter and its published mark": {func(s *matrixScene, r Message) {
			s.landDirect(r)
			s.writeMark(r, oncePublished)
		}, DispositionPublished},
		"the published mark alone": {func(s *matrixScene, r Message) {
			s.writeMark(r, oncePublished)
		}, DispositionPublished},
		"an intent with no letter": {func(s *matrixScene, r Message) {
			s.writeMark(r, onceIntent)
		}, DispositionMoot},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			t.Run("a stop-only pass first", func(t *testing.T) {
				t.Parallel()
				s, r := endedWithEvidence(t, c.returns)
				if err := s.stopOnly(); err != nil {
					t.Fatalf("the stop-only pass: %v", err)
				}
				if open := stopsOnRecord(t, s.dir, "api"); len(open) != 0 {
					t.Fatalf("the stop-only pass left %v open", open)
				}
				s.checkDisposition(t, r, c.disposition, s.barrier)
			})
			// Every durable step of the pass that resolves, killed there; the
			// next barrier then runs in a process that is not.
			for pass, run := range map[string]func(*matrixScene) error{
				"the stop-only pass": (*matrixScene).stopOnly,
				"the barrier":        (*matrixScene).barrier,
			} {
				t.Run(pass+" killed at each step", func(t *testing.T) {
					t.Parallel()
					s, _ := endedWithEvidence(t, c.returns)
					s.access.arm(true)
					_ = run(s)
					s.access.arm(false)
					steps := s.access.steps()
					if len(steps) == 0 {
						t.Fatal("the pass made no durable step")
					}
					resolved := false
					for at, step := range steps {
						resolved = resolved || filepath.Ext(step) == resolvedSuffix
						s, r := endedWithEvidence(t, c.returns)
						s.access.cut, s.access.flush = at+1, false
						cut := func() error {
							s.access.arm(true)
							_ = run(s)
							s.access.arm(false)
							if !s.access.lift() {
								t.Fatalf("the cut at %d (%s) never fired", at+1, step)
							}
							return nil
						}
						s.checkDisposition(t, r, c.disposition, cut, s.barrier)
					}
					if !resolved {
						t.Errorf("no step of %s resolves the occurrence: %v", pass, steps)
					}
				})
			}
		})
	}
}

// endedWithEvidence is a room whose report R is held by an occurrence, its
// run ended, and what returns set in the recipient's mailbox.
func endedWithEvidence(t *testing.T, returns func(*matrixScene, Message)) (*matrixScene, Message) {
	t.Helper()
	s, r := newContractScene(t)
	s.endR()
	returns(s, r)
	return s, r
}

// stopOnly is the stop-only pass a call that would change api's mailbox
// makes: the plan and its resolutions, no journal.
func (s *matrixScene) stopOnly() error {
	return state.WithMailboxLock(context.Background(), s.dir, "api", func() error { return MailboxStopped(s.dir, "api") })
}

// checkDisposition runs passes and holds what J's saves recorded of R, read
// before each step, since its done form keeps none of it: R recorded as
// disposition, never as the other, and every occurrence resolved once the
// last pass, which must succeed, is over.
func (s *matrixScene) checkDisposition(t *testing.T, r Message, disposition string, passes ...func() error) {
	t.Helper()
	var published, moot bool
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
	var err error
	for _, pass := range passes {
		err = pass()
	}
	s.access.mu.Lock()
	s.access.beforeOp = nil
	s.access.mu.Unlock()
	if err != nil {
		t.Fatalf("the barrier: %v", err)
	}
	if _, err := os.Stat(open + doneSuffix); err != nil {
		t.Fatalf("J is not done: %v", err)
	}
	want := disposition == DispositionPublished
	if published != want || moot == want {
		t.Errorf("J recorded R published %v, moot %v; want %s", published, moot, disposition)
	}
	if open := stopsOnRecord(t, s.dir, "api"); len(open) != 0 {
		t.Errorf("occurrences still open: %v", open)
	}
}

// A report no occurrence was ever about keeps the ordinary rule: its
// publication's save lost and its run ended, it is moot, whatever the
// recipient's mailbox shows, since nothing held it to its evidence.
func TestAReportNeverHeldIsMootOnceItsRunEnded(t *testing.T) {
	s := newMatrixScene(t, false)
	r := s.report
	s.landDirect(r)
	s.writeMark(r, oncePublished)
	s.endR()
	if open := stopsOnRecord(t, s.dir, "api"); len(open) != 0 {
		t.Fatalf("setup: occurrences open: %v", open)
	}
	s.checkDisposition(t, r, DispositionMoot, s.barrier)
}

// A resolved occurrence whose record no longer reads may hold a report, so
// the barrier stops on it rather than settle any report by the ordinary rule.
func TestAResolvedRecordThatDoesNotReadStopsTheBarrier(t *testing.T) {
	s, r := endedWithEvidence(t, func(s *matrixScene, r Message) {
		s.landDirect(r)
		s.writeMark(r, oncePublished)
	})
	if err := s.stopOnly(); err != nil {
		t.Fatalf("the stop-only pass: %v", err)
	}
	resolutions, _ := filepath.Glob(filepath.Join(stopsPath(s.dir, "api"), "*", "*"+resolvedSuffix))
	if len(resolutions) != 1 {
		t.Fatalf("setup: resolutions %v", resolutions)
	}
	writeRaw(t, strings.TrimSuffix(resolutions[0], resolvedSuffix), "{")
	if err := s.barrier(); err == nil {
		t.Fatal("the barrier settled R past a resolved record that does not read")
	}
	if _, err := os.Stat(filepath.Join(JournalPath(s.dir, "api"), "end"+doneSuffix)); err == nil {
		t.Error("J is done")
	}
	if saved, err := readJournalFile(filepath.Join(JournalPath(s.dir, "api"), "end")); err != nil || len(saved.Moot)+len(saved.Published) > 0 {
		t.Errorf("J recorded %v, %v (%v) of R %s", saved.Published, saved.Moot, err, r.ID)
	}
}
