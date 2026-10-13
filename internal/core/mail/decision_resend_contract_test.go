package mail

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	dm "github.com/praline-labs/rewake/internal/core/mail/decisionmodel"
)

// A resend publishes by S3's contract, as any report does (E8,
// docs/v2/stage3-decision-model.md#the-expected-outcome-of-a-world): r ended
// before an attempt read it live leaves the copy moot with nothing written;
// the attempt that read r live publishes though r ends inside its section,
// and its first write across r's end lands once; a lost save of Published is
// recovered as moot, the copy landed; the next run's sweep waits for the
// attempt in the section; an unknown at any read is an occurrence about the
// copy's operation and a subject of its own, which r's end leaves open —
// evidence may still close it, and only the person may decide it.
func TestDecisionResendKeepsPublicationContract(t *testing.T) {
	paths := map[string]struct {
		life        string
		loseSave    bool
		written     int
		disposition string
	}{
		"r ended before the first attempt read it":  {dm.REndsBeforeLive, false, 0, DispositionMoot},
		"r ending inside the section":               {dm.REndsInSection, false, 1, DispositionPublished},
		"a first write across r's end":              {dm.REndsToMark, false, 1, DispositionPublished},
		"r ending before the save":                  {dm.REndsToSave, false, 1, DispositionPublished},
		"a lost save across r's end, recovered":     {dm.REndsToSave, true, 1, DispositionMoot},
		"r alive throughout":                        {dm.RAlive, false, 1, DispositionPublished},
		"r ending as the resend finishes, after it": {dm.REndsToTail, false, 1, DispositionPublished},
	}
	for name, c := range paths {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s, r := newContractScene(t)
			path := newResendPath(s, r, dm.NoneThenPublished, c.life)
			lost := false
			s.access.mu.Lock()
			s.access.onStat = path.stat
			s.access.beforeOp = func(op, at string) error {
				if err := path.before(op, at); err != nil {
					return err
				}
				path.mu.Lock()
				defer path.mu.Unlock()
				if c.loseSave && !lost && path.letter && op == "write" && strings.Contains(at, "journal"+string(filepath.Separator)+resendPrefix) {
					lost = true
					return &fs.PathError{Op: op, Path: at, Err: syscall.EIO}
				}
				return nil
			}
			s.access.mu.Unlock()
			_, _ = s.decide(r, ChoiceNotArrived, false)
			if c.loseSave {
				if !lost {
					t.Fatal("the save of Published was never tried")
				}
				if err := s.barrier(); err != nil {
					t.Fatalf("the barrier after the lost save: %v", err)
				}
			}
			path.checkPlaced(t)
			s.checkCopy(t, r, path.copied, c.written, c.disposition)
		})
	}

	t.Run("the next run's sweep waits for the attempt in the section", func(t *testing.T) {
		s, r := newContractScene(t)
		var once sync.Once
		inSection, release := make(chan struct{}), make(chan struct{})
		var copied string
		s.access.mu.Lock()
		s.access.beforeOp = func(op, at string) error {
			if copied == "" && op == "publish" && strings.Contains(at, string(filepath.Separator)+resendPrefix) {
				copied = s.resentOf(r)
				// An earlier attempt wrote the letter and lost its save.
				prime := Message{ID: copied, From: "api", FromEpoch: s.epoch, To: "web", ToEpoch: r.ToEpoch, Kind: Finished, Text: "the answer", Resends: r.ID}
				s.landDirect(prime)
				s.writeMark(prime, onceIntent)
			}
			return nil
		}
		s.access.onStat = func(at string, plan bool) {
			if !plan && copied != "" && filepath.Base(at) == copied+".json" {
				once.Do(func() {
					close(inSection)
					<-release
				})
			}
		}
		s.access.mu.Unlock()
		decided := make(chan error, 1)
		go func() {
			_, err := s.decide(r, ChoiceNotArrived, false)
			decided <- err
		}()
		waitFor(t, inSection, "the retry reaching the section")
		s.endR()
		next := startRoleRun(t, s.dir, "web", "")
		swept := make(chan struct{})
		go func() {
			defer close(swept)
			sweepAsLiveRun(t, s.dir, "web", next.session.Epoch())
		}()
		select {
		case <-swept:
			t.Fatal("the next run swept inside the attempt's section")
		case <-time.After(300 * time.Millisecond):
		}
		close(release)
		if err := waitErr(t, decided, "the decision"); err != nil {
			t.Fatalf("the decision: %v", err)
		}
		waitFor(t, swept, "the next run's sweep")
		s.checkCopy(t, r, copied, 1, DispositionPublished)
	})

	unknowns := map[string]func(s *matrixScene, copied Message){
		"its intent unreadable": func(s *matrixScene, copied Message) { s.writeMark(copied, "damaged") },
		"its mark unreadable beside it": func(s *matrixScene, copied Message) {
			s.landDirect(copied)
			s.writeMark(copied, "damaged")
		},
		"its letter's place unsearchable": func(s *matrixScene, copied Message) {
			s.writeMark(copied, onceIntent)
			s.faultLetter(copied.ID)
		},
		"r's mailbox unsearchable": func(s *matrixScene, copied Message) {
			s.writeMark(copied, onceIntent)
			s.fault(true, false, copied)
		},
	}
	for name, setUp := range unknowns {
		t.Run("unknown: "+name, func(t *testing.T) {
			t.Parallel()
			s, r, copied := s3UnknownCopy(t, setUp)
			before := s.access.written(copied.ID)
			s.checkHeld(t, r, copied, before)
		})
	}

	for _, settle := range []string{"evidence returns", "the person decides"} {
		t.Run("an open occurrence past r's end: "+settle, func(t *testing.T) {
			t.Parallel()
			s, r, copied := s3UnknownCopy(t, func(s *matrixScene, copied Message) { s.writeMark(copied, "damaged") })
			s.endR()
			s.checkHeld(t, r, copied, 0)
			if _, err := s.decide(copied, ChoiceArrived, true); err == nil || fileExists(DecisionPath(s.dir, "api", copied.ID)) {
				t.Fatalf("r decided its copy after its end: %v", err)
			}
			disposition := DispositionPublished
			if settle == "evidence returns" {
				s.landDirect(copied)
				s.writeMark(copied, oncePublished)
				if err := s.barrier(); err != nil {
					t.Fatalf("the barrier once the evidence returned: %v", err)
				}
			} else {
				disposition = DispositionDecided
				if _, err := s.decide(copied, ChoiceArrived, false); err != nil {
					t.Fatalf("the person's decision: %v", err)
				}
			}
			s.checkCopy(t, r, copied.ID, s.access.written(copied.ID), disposition)
			if n := s.access.written(copied.ID); n > 1 {
				t.Errorf("the copy was written %d times", n)
			}
		})
	}
}

// newContractScene is a room whose report R is unknown, r a process of its
// own, so its end is proven.
func newContractScene(t *testing.T) (*matrixScene, Message) {
	t.Helper()
	s := newMatrixScene(t, false)
	r := s.report
	s.writeMark(r, "damaged")
	if err := s.barrier(); err == nil {
		t.Fatal("the barrier went past R's unknown outcome")
	}
	return s, r
}

// s3UnknownCopy has the person decide R "did not arrive" with R'’s place
// set up as setUp says when R' is installed, before its first attempt.
func s3UnknownCopy(t *testing.T, setUp func(*matrixScene, Message)) (*matrixScene, Message, Message) {
	t.Helper()
	s, r := newContractScene(t)
	var copied Message
	s.access.mu.Lock()
	s.access.beforeOp = func(op, at string) error {
		if copied.ID == "" && op == "publish" && strings.Contains(at, string(filepath.Separator)+resendPrefix) {
			copied = Message{ID: s.resentOf(r), From: "api", FromEpoch: s.epoch, To: "web", ToEpoch: r.ToEpoch, Kind: Finished, Text: "the answer", Resends: r.ID}
			setUp(s, copied)
		}
		return nil
	}
	s.access.mu.Unlock()
	// The decision is made; its barrier stops on the copy's unknown.
	_, _ = s.decide(r, ChoiceNotArrived, false)
	if copied.ID == "" {
		t.Fatal("the resend was never installed")
	}
	return s, r, copied
}

// checkHeld holds a copy whose outcome is unknown: the barrier stops on it,
// an occurrence about it alone is open and it is a subject of its own, J'
// is not done, R's waits stand, and nothing more was written.
func (s *matrixScene) checkHeld(t *testing.T, r, copied Message, written int) {
	t.Helper()
	if err := s.barrier(); err == nil {
		t.Fatal("the barrier went past the copy's unknown outcome")
	}
	s.views = nil
	o := s.outcome(t, r)
	if len(o.open) != 1 || o.open[0] != copied.ID {
		t.Errorf("open occurrences %v; the copy %s alone is unknown", o.open, copied.ID)
	}
	if o.waits != 1 || o.closure != nil {
		t.Errorf("R's waits %d, closure %+v, with its resend unknown", o.waits, o.closure)
	}
	if subject, err := FindSubject(s.dir, "api", copied.ID); err != nil || subject.Report != copied.ID {
		t.Errorf("the copy as a subject: %+v, %v", subject, err)
	}
	if summary := s.resendSummary(t, r); summary != nil {
		t.Errorf("the resend finished on an unknown: %+v", summary)
	}
	if n := s.access.written(copied.ID); n != written {
		t.Errorf("the copy was written %d times while unknown, %d before", n, written)
	}
}

// checkCopy holds R'’s end: written as many times as the path says, its
// resend done with the disposition it says, R's step closed and its waits
// cleared, nothing left open, and R never written.
func (s *matrixScene) checkCopy(t *testing.T, r Message, copied string, written int, disposition string) {
	t.Helper()
	if n := s.access.written(copied); n != written {
		t.Errorf("the copy was written %d times, want %d", n, written)
	}
	if n := s.access.written(r.ID); n != 0 {
		t.Errorf("R was written %d times", n)
	}
	summary := s.resendSummary(t, r)
	if summary == nil || summary.Report != copied || summary.Disposition != disposition {
		t.Errorf("the resend's summary %+v, want %s of %s", summary, disposition, copied)
	}
	s.views = nil
	if o := s.outcome(t, r); o.waits != 0 || o.closure == nil || len(o.open) != 0 {
		t.Errorf("waits %d, closure %+v, open %v", o.waits, o.closure, o.open)
	}
}

// resendSummary is what the done form of R's resend keeps, nil while it is
// not done.
func (s *matrixScene) resendSummary(t *testing.T, r Message) *ResendSummary {
	t.Helper()
	raw, err := os.ReadFile(DecisionPath(s.dir, "api", r.ID))
	if err != nil {
		t.Fatalf("the decision on R: %v", err)
	}
	d, err := parseDecision(DecisionPath(s.dir, "api", r.ID), raw)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := readJournalFile(filepath.Join(JournalPath(s.dir, "api"), ResendJournal(d.ID)+doneSuffix))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatalf("the done resend: %v", err)
	}
	return journal.Summary
}

func waitFor(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatalf("%s never came", what)
	}
}

func waitErr(t *testing.T, ch <-chan error, what string) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(10 * time.Second):
		t.Fatalf("%s never finished", what)
		return nil
	}
}
