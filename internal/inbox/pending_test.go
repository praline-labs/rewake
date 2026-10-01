package inbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

func mark(t *testing.T, dir, epoch, text string, at int64) string {
	t.Helper()
	file := MarkName(at, NewID())
	if err := MarkPending(dir, "api", epoch, file, text, at); err != nil {
		t.Fatal(err)
	}
	return file
}

func within(t *testing.T, dir, epoch string, start, ended int64) (string, bool) {
	t.Helper()
	found, ok, err := MarkWithin(dir, "api", epoch, start, ended)
	if err != nil {
		t.Fatal(err)
	}
	return found.Text, ok
}

// A mark decides every end whose window holds it, as often as it is asked:
// no turn end removes a mark, so a retry of an earlier end still finds the
// mark a later end also saw (docs/turn-end-recovery.md#pending-marks).
func TestAMarkIsNeverTakenAway(t *testing.T) {
	dir := stateDir(t)
	if _, ok := within(t, dir, "e1", 100, 200); ok {
		t.Fatal("a turn end without a mark was pending")
	}
	mark(t, dir, "e1", "the suite is running", 150)
	for range 2 {
		if text, ok := within(t, dir, "e1", 100, 200); !ok || text != "the suite is running" {
			t.Fatalf("the marked turn end = %q %v", text, ok)
		}
	}
}

// The window runs from just after its start to its end, inclusive: a mark at
// the very start is the turn before's, one at the very end is this turn's.
func TestTheWindowOpensAfterItsStartAndClosesAtItsEnd(t *testing.T) {
	for _, c := range []struct {
		at   int64
		want bool
	}{{99, false}, {100, false}, {101, true}, {200, true}, {201, false}} {
		dir := stateDir(t)
		mark(t, dir, "e1", "waiting", c.at)
		if _, ok := within(t, dir, "e1", 100, 200); ok != c.want {
			t.Errorf("a mark at %d in (100, 200]: %v, want %v", c.at, ok, c.want)
		}
	}
}

// Where the turn's end is not known, the mark cannot be tied to it; a start
// of zero is before the run's first mark.
func TestAnEndWithNoTimeTakesNoMark(t *testing.T) {
	dir := stateDir(t)
	mark(t, dir, "e1", "x", 150)
	if _, ok := within(t, dir, "e1", 100, 0); ok {
		t.Error("a turn with no known end honored the mark")
	}
	if text, ok := within(t, dir, "e1", 0, 200); !ok || text != "x" {
		t.Errorf("a window from the run's start missed its mark: %q %v", text, ok)
	}
}

// A mark of another run of the name is never honored: it is kept with that
// run's records and swept with them.
func TestAMarkOfAnotherRunIsIgnoredAndSwept(t *testing.T) {
	dir := stateDir(t)
	mark(t, dir, "e1", "old run", 150)
	mark(t, dir, "e2", "this run", 160)
	if text, ok := within(t, dir, "e2", 100, 200); !ok || text != "this run" {
		t.Errorf("got %q %v", text, ok)
	}
	sweepMarks(dir, "api", "e2")
	if _, ok := within(t, dir, "e1", 100, 200); ok {
		t.Error("the sweep kept an ended run's mark")
	}
	if _, ok := within(t, dir, "e2", 100, 200); !ok {
		t.Error("the sweep took the live run's mark")
	}
}

// Marked twice in one turn, the later is the one sent; two of the same time
// are ordered by name, whichever was written first.
func TestTheLatestMarkInATurnWins(t *testing.T) {
	dir := stateDir(t)
	mark(t, dir, "e1", "first", 150)
	mark(t, dir, "e1", "second", 160)
	if text, ok := within(t, dir, "e1", 100, 200); !ok || text != "second" {
		t.Errorf("got %q %v", text, ok)
	}
	for _, order := range [][]string{{"b", "a"}, {"a", "b"}} {
		dir := stateDir(t)
		for _, id := range order {
			if err := MarkPending(dir, "api", "e1", MarkName(150, "0-"+id), "mark "+id, 150); err != nil {
				t.Fatal(err)
			}
		}
		if text, ok := within(t, dir, "e1", 100, 200); !ok || text != "mark b" {
			t.Errorf("written %v: %q %v, want the name that sorts last", order, text, ok)
		}
	}
}

// A retry of one call finds its mark and writes nothing; a mark with no time
// or a name that does not say its time is refused.
func TestAMarkIsWrittenOncePerCall(t *testing.T) {
	dir := stateDir(t)
	file := mark(t, dir, "e1", "first attempt", 150)
	if err := MarkPending(dir, "api", "e1", file, "the retry", 150); err != nil {
		t.Fatal(err)
	}
	if text, _ := within(t, dir, "e1", 100, 200); text != "first attempt" {
		t.Errorf("a retry wrote over its mark: %q", text)
	}
	if err := MarkPending(dir, "api", "e1", MarkName(0, NewID()), "x", 0); err == nil {
		t.Error("a mark with no time was written")
	}
	if err := MarkPending(dir, "api", "e1", MarkName(140, NewID()), "x", 150); err == nil {
		t.Error("a mark whose name says another time was written")
	}
}

// A mark in the window that cannot be read may be this turn's: the end stops.
// One outside the window is another turn's and does not matter.
func TestAnUnreadableMarkInTheWindowIsAnError(t *testing.T) {
	dir := stateDir(t)
	file := mark(t, dir, "e1", "x", 150)
	path, _ := marksPath(dir, "api", "e1")
	if err := os.WriteFile(filepath.Join(path, file), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := MarkWithin(dir, "api", "e1", 100, 200); err == nil {
		t.Error("an unreadable mark in the window was passed over")
	}
	if _, _, err := MarkWithin(dir, "api", "e1", 150, 200); err != nil {
		t.Errorf("an unreadable mark outside the window stopped the end: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, "stray"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := MarkWithin(dir, "api", "e1", 150, 200); err == nil {
		t.Error("a file that names no mark was passed over")
	}
}

// The mark lives apart from the mail: a mailbox lists no message for it.
func TestPendingFilesAreNotMail(t *testing.T) {
	dir := stateDir(t)
	mark(t, dir, "e1", "x", 150)
	entries, err := os.ReadDir(state.InboxPath(dir, "api"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			t.Errorf("a file in the mailbox itself: %s", entry.Name())
		}
	}
}

// An interim report owes nothing and settles nothing: it is a report for
// keeping it readable, but no question takes it for its answer.
func TestAnInterimReportSettlesNothing(t *testing.T) {
	interim := Message{Kind: Interim, FromEpoch: "e", InReplyTo: []string{"q1"}}
	if Owed(interim) || !IsReport(interim) || Settles(interim) || answers(interim, "q1") {
		t.Errorf("owed %v report %v settles %v answers %v", Owed(interim), IsReport(interim), Settles(interim), answers(interim, "q1"))
	}
	finished := Message{Kind: Finished, FromEpoch: "e", InReplyTo: []string{"q1"}}
	if !Settles(finished) || !answers(finished, "q1") {
		t.Error("a finished report stopped settling")
	}
}

// A question waiting in this session does not swallow an interim report: it
// is announced like any other mail, and the question keeps waiting.
func TestAnInterimReportIsNotTakenByAWaitingQuestion(t *testing.T) {
	dir := stateDir(t)
	marks := state.AnsweringPath(dir, "api")
	if err := os.MkdirAll(marks, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(marks, "q1"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	interim := message("still running")
	interim.Kind, interim.InReplyTo = Interim, []string{"q1"}
	if err := Put(dir, interim); err != nil {
		t.Fatal(err)
	}
	notices := 0
	server := &Server{Dir: dir, Name: "api", Deliver: func(context.Context, Message) Result {
		notices++
		return Result{State: Delivered, Via: "socket"}
	}}
	server.attempts, server.outcomes = map[string]time.Time{}, map[string]Result{}
	server.drain(context.Background())
	if notices != 1 {
		t.Errorf("notices = %d, want the interim report announced", notices)
	}
}

// The window opens after the later of the turn's start and the latest earlier
// end of the same run a journal records, done or not, so a start that failed
// to be recorded widens nothing; another run's ends, and this run's later
// ones, move nothing. A journal that cannot be read may be the earlier end,
// and is an error.
func TestTheWindowOpensAfterTheRunsLatestEarlierEnd(t *testing.T) {
	dir := stateDir(t)
	for id, journal := range map[string]TurnJournal{
		"earlier": {Epoch: "e1", Op: "earlier", Ended: 150},
		"other":   {Epoch: "e2", Op: "other", Ended: 180},
		"later":   {Epoch: "e1", Op: "later", Ended: 250},
	} {
		if err := WriteJournal(dir, "api", id, journal); err != nil {
			t.Fatal(err)
		}
	}
	if err := live(dir).finishJournal(context.Background(), "api", "earlier"); err != nil {
		t.Fatal(err)
	}
	for _, started := range []int64{0, 100, 150} {
		if start, err := TurnWindowStart(dir, "api", "e1", started, 200); err != nil || start != 150 {
			t.Errorf("started %d: the window opens at %d (%v), want 150", started, start, err)
		}
	}
	if start, _ := TurnWindowStart(dir, "api", "e1", 160, 200); start != 160 {
		t.Errorf("a recorded start after the earlier end opens it at %d, want 160", start)
	}
	if err := os.WriteFile(filepath.Join(JournalPath(dir, "api"), "broken"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := TurnWindowStart(dir, "api", "e1", 100, 200); err == nil {
		t.Error("an unreadable journal was passed over")
	}
}
