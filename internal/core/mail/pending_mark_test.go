package mail

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/boottime"
	"github.com/praline-labs/rewake/internal/infra/state"
)

func (lab endLab) box() Box { return Open(lab.dir, "api", lab.self.Epoch()) }

// marked says whether api's run holds any pending mark.
func (lab endLab) marked(t *testing.T) bool {
	t.Helper()
	_, ok, err := lab.box().MarkWithin(0, math.MaxInt64)
	if err != nil {
		t.Fatal(err)
	}
	return ok
}

// A mark of a turn that owes a report names who waits, and the turn's end
// finds it.
func TestAMarkNamesWhoWaitsForTheTurn(t *testing.T) {
	lab := newEndLab(t)
	waiting, err := lab.box().MarkTurnPending(t.Context(), "the suite is running", boottime.Now())
	if err != nil || len(waiting) != 1 || waiting[0] != "web" {
		t.Fatalf("the mark answered %v, %v", waiting, err)
	}
	if !lab.marked(t) {
		t.Fatal("no mark was written")
	}
}

// A mark made after its turn's end is on record writes nothing: the end
// reported without it, and no later end's window may take it.
func TestAMarkAfterTheTurnEndedWritesNothing(t *testing.T) {
	lab := newEndLab(t)
	at := boottime.Now()
	end := lab.end(t, "turn-1")
	end.Ended = at + 1000
	if err := lab.complete(t, end); err != nil {
		t.Fatal(err)
	}
	lab.read(t, "another task")
	if _, err := lab.box().MarkTurnPending(t.Context(), "too late", at); !errors.Is(err, ErrTurnEnded) {
		t.Fatalf("a mark after the end answered %v", err)
	}
	if lab.marked(t) {
		t.Fatal("a mark after the end was written")
	}
}

// A turn start recorded after the mark's time proves its turn ended, though
// no end was heard.
func TestALaterTurnStartRefusesTheMark(t *testing.T) {
	lab := newEndLab(t)
	at := boottime.Now()
	if err := lab.box().RecordTurnStart(at + 1); err != nil {
		t.Fatal(err)
	}
	if _, err := lab.box().MarkTurnPending(t.Context(), "too late", at); !errors.Is(err, ErrTurnEnded) {
		t.Fatalf("a mark before a later start answered %v", err)
	}
	if lab.marked(t) {
		t.Fatal("a mark of an ended turn was written")
	}
}

// With nobody waiting there is nothing to keep open, and no mark is written.
func TestAMarkOwingNobodyIsNotWritten(t *testing.T) {
	lab := newEndLab(t)
	if err := lab.complete(t, lab.end(t, "turn-1")); err != nil {
		t.Fatal(err)
	}
	if _, err := lab.box().MarkTurnPending(t.Context(), "nothing owed", boottime.Now()+1000); !errors.Is(err, ErrNothingOwed) {
		t.Fatalf("a mark owing nobody answered %v", err)
	}
	if lab.marked(t) {
		t.Fatal("a mark owing nobody was written")
	}
}

// A record a mark is decided by that cannot be read writes no mark, and the
// failure names it: the turn starts, or the journals, which may hold this
// turn's end.
func TestAnUnreadableRecordWritesNoMark(t *testing.T) {
	for step, closed := range map[Step]func(lab endLab) string{
		StepTurnStart: func(lab endLab) string {
			if err := lab.box().RecordTurnStart(1); err != nil {
				t.Fatal(err)
			}
			path, _ := awaitingPath(lab.dir, "api", lab.self.Epoch())
			return filepath.Join(path, turnStartsDir)
		},
		StepEnded: func(lab endLab) string {
			path := JournalPath(lab.dir, "api")
			if err := state.EnsureSubdir(path); err != nil {
				t.Fatal(err)
			}
			return path
		},
	} {
		lab := newEndLab(t)
		path := closed(lab)
		if err := os.Chmod(path, 0); err != nil {
			t.Fatal(err)
		}
		_, err := lab.box().MarkTurnPending(t.Context(), "unknown", boottime.Now())
		if err := os.Chmod(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if stepOf(err) != step {
			t.Errorf("with %s closed the mark answered %v, want step %d", path, err, step)
		}
		if lab.marked(t) {
			t.Errorf("with %s closed a mark was written", path)
		}
	}
}
