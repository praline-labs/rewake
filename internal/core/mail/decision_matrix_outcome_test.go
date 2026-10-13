package mail

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// matrixCut is the durable step a run of a world cuts: none (clean), or the
// at-th, killed there or with its flush failed.
type matrixCut struct {
	at    int
	flush bool
}

// matrixMoment is the world at one barrier after D: whether r lived and its
// mailbox could be searched. What becomes of a copy or a note sent after D
// follows from these alone, never from what the code recorded.
type matrixMoment struct{ alive, searchable bool }

func (s *matrixScene) afterD(ran bool) {
	if ran {
		s.moments = append(s.moments, matrixMoment{alive: !s.r.ended, searchable: !s.unsearchable})
	}
}

// matrixFate is what becomes of a copy sent after D: landed, or held on an
// unknown nothing settled — and otherwise moot, nothing written.
type matrixFate struct{ landed, held bool }

// resendOutcome follows the copy D sends through the barriers after it: the
// first that reads r live publishes it, when it can search r's mailbox, and
// turns it unknown when it cannot; the first that finds r ended makes it
// moot, unless it turned unknown before, which then holds it (S3).
func (s *matrixScene) resendOutcome() matrixFate {
	held := false
	for _, m := range s.moments {
		switch {
		case m.alive && m.searchable:
			return matrixFate{landed: true}
		case !m.alive && !held:
			return matrixFate{}
		case m.alive:
			held = true
		}
	}
	return matrixFate{held: held}
}

// noteFate follows a note to r: written at the first barrier that finds r
// live with a mailbox it can search, told run-ended at the first that finds
// r's process gone, and owed while neither.
func (s *matrixScene) noteFate() string {
	for _, m := range s.moments {
		switch {
		case m.alive && m.searchable:
			return ToldWritten
		case !m.alive:
			return ToldEnded
		}
	}
	return ""
}

// matrixOutcome is what the room holds at the end of a world.
type matrixOutcome struct {
	decisions int
	decision  *Decision
	resent    string
	waits     int
	closure   *Closure
	open      []string
}

func (s *matrixScene) outcome(t *testing.T, r Message) matrixOutcome {
	t.Helper()
	var o matrixOutcome
	if s.views == nil {
		views, err := Decisions(s.dir, "api")
		if err != nil {
			t.Fatalf("the decisions: %v", err)
		}
		s.views = views
	}
	for _, view := range s.views {
		if view.Report == r.ID {
			o.decisions++
			o.decision = view.Decision
		}
	}
	if o.decision != nil && o.decision.Choice == ChoiceNotArrived {
		resend, err := o.decision.resendJournal()
		if err != nil || len(resend.Reports) != 1 {
			t.Fatalf("the resend: %+v %v", resend, err)
		}
		o.resent = resend.Reports[0].ID
	}
	waits, err := ReadWaiters(s.dir, "api", s.epoch)
	if err != nil {
		t.Fatalf("the waits: %v", err)
	}
	o.waits = len(waits)
	path := DecisionPath(s.dir, "api", r.ID) + closedSuffix
	if raw, err := os.ReadFile(path); err == nil {
		closure, err := parseClosure(path, raw)
		if err != nil {
			t.Fatalf("the closure: %v", err)
		}
		o.closure = &closure
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	for _, stop := range stopsOnRecord(t, s.dir, "api") {
		// An operation names its report last; a cause with none, by its path.
		about := stop.record.Op
		if about == "" && len(stop.record.Paths) > 0 {
			about = filepath.Base(stop.record.Paths[0])
		}
		o.open = append(o.open, about[strings.LastIndex(about, "/")+1:])
	}
	return o
}

// toldEither is the fate of a note whose telling a cut may have moved past
// r's end: written once and told so, or told run-ended — after a landing
// whose told record the cut lost, too ("A crash before that record repeats a
// publication that answers written before, or ended",
// docs/v2/stage3-decision-recovery.md).
const toldEither = "written-or-ended"

// checkNotes holds the notes of d to the model: one to r unless r decided
// it, verified, and one to the room's main, each written once to a live run
// whose mailbox can be searched, told run-ended to an ended one, and owed
// otherwise.
func (s *matrixScene) checkNotes(t *testing.T, d Decision, verified bool, recipient string) {
	t.Helper()
	want := map[string]string{ToldMain: ToldWritten}
	if !verified {
		want[ToldRecipient] = recipient
	}
	if len(d.Notes) != len(want) {
		t.Errorf("the decision owes %d notes; the model %d", len(d.Notes), len(want))
	}
	for _, entry := range d.Notes {
		fate, ok := want[entry.Told]
		if !ok {
			t.Errorf("a note the model does not owe: %s", entry.Told)
			continue
		}
		told, err := readTold(osAccess{}, toldPath(s.dir, "api", d.Subject.Report, entry.Told), entry)
		n := s.access.written(entry.Note.ID)
		switch fate {
		case "":
			if !errors.Is(err, os.ErrNotExist) || n != 0 {
				t.Errorf("the note to %s, owed, is told %+v (%v), written %d times", entry.Told, told, err, n)
			}
		case toldEither:
			if err != nil || n > 1 || told.Answer != ToldEnded && n != 1 {
				t.Errorf("the note to %s is told %+v (%v), written %d times", entry.Told, told, err, n)
			}
		case ToldEnded:
			if err != nil || told.Answer != ToldEnded || n != 0 {
				t.Errorf("the note to the ended %s is told %+v (%v), written %d times", entry.Told, told, err, n)
			}
		default:
			if err != nil || told.Answer == ToldEnded || n != 1 {
				t.Errorf("the note to %s is told %+v (%v), written %d times", entry.Told, told, err, n)
			}
		}
	}
}
