package inbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

func take(t *testing.T, dir, epoch string, started, ended int64) (string, bool) {
	t.Helper()
	text, ok, err := TakePending(dir, "api", epoch, started, ended)
	if err != nil {
		t.Fatal(err)
	}
	return text, ok
}

func mark(t *testing.T, dir, epoch, text string, at int64) {
	t.Helper()
	if err := MarkPending(dir, "api", epoch, text, at); err != nil {
		t.Fatal(err)
	}
}

func marked(dir string) bool {
	_, err := os.Stat(pendingPath(dir, "api"))
	return err == nil
}

// A mark made within a turn holds for that turn's end, once.
func TestAPendingMarkHoldsForItsOwnTurn(t *testing.T) {
	dir := stateDir(t)
	if _, ok := take(t, dir, "e1", 100, 200); ok {
		t.Fatal("a turn end without a mark was pending")
	}
	mark(t, dir, "e1", "the suite is running", 150)
	if text, ok := take(t, dir, "e1", 100, 200); !ok || text != "the suite is running" {
		t.Fatalf("the marked turn end = %q %v", text, ok)
	}
	if _, ok := take(t, dir, "e1", 100, 200); ok || marked(dir) {
		t.Fatal("the mark reached a second turn end")
	}
}

// A mark from a turn whose end never reached rewake — the person pressed Esc,
// or the hook's payload never came — does not reach the next turn: that turn
// started after the mark, so its end is a report, and the mark is gone.
func TestAMarkFromAnInterruptedTurnIsAReportNext(t *testing.T) {
	dir := stateDir(t)
	mark(t, dir, "e1", "waiting", 150)
	// No turn end for the turn it was made in; the next turn runs 300..400.
	if _, ok := take(t, dir, "e1", 300, 400); ok {
		t.Error("a mark from an earlier turn made this turn end interim")
	}
	if marked(dir) {
		t.Error("the stale mark was left behind")
	}
}

// Codex publishes turn ends late. A mark made in turn K+1 while K's end is
// still being published belongs to K+1: K's end is a report and leaves the mark
// where it is, and K+1's end takes it.
func TestALatePublishedTurnLeavesALaterMark(t *testing.T) {
	dir := stateDir(t)
	mark(t, dir, "e1", "K+1 waits", 250) // made during K+1 (220..300)
	if _, ok := take(t, dir, "e1", 100, 200); ok || !marked(dir) {
		t.Fatal("turn K took the mark made in K+1")
	}
	if text, ok := take(t, dir, "e1", 220, 300); !ok || text != "K+1 waits" {
		t.Fatalf("turn K+1 = %q %v", text, ok)
	}
}

// Where the turn's start or end is not known, the mark cannot be tied to it:
// the turn end is a report. An unknown start removes the mark unless it is a
// later turn's; an unknown end keeps it, since it might be.
func TestAnUnknownTurnIsAReport(t *testing.T) {
	dir := stateDir(t)
	mark(t, dir, "e1", "x", 150)
	if _, ok := take(t, dir, "e1", 0, 200); ok || marked(dir) {
		t.Error("a turn with no known start honored the mark, or kept it")
	}
	mark(t, dir, "e1", "x", 150)
	if _, ok := take(t, dir, "e1", 100, 0); ok || !marked(dir) {
		t.Error("a turn with no known end honored the mark, or dropped it")
	}
}

// A late UserPromptSubmit of the current turn records an earlier time than the
// mark and cannot move the start past it.
func TestALateTurnStartOfTheSameTurnKeepsTheMark(t *testing.T) {
	dir := stateDir(t)
	mark(t, dir, "e1", "x", 150)
	// The start the late hook records is its process start, 110: before the mark.
	if text, ok := take(t, dir, "e1", 110, 200); !ok || text != "x" {
		t.Errorf("got %q %v", text, ok)
	}
}

// A mark of an earlier run of the name is never honored.
func TestAMarkOfAnEarlierRunIsIgnored(t *testing.T) {
	dir := stateDir(t)
	mark(t, dir, "e1", "old run", 150)
	if _, ok := take(t, dir, "e2", 100, 200); ok || marked(dir) {
		t.Error("a mark of an earlier run was honored, or kept")
	}
}

// Marked twice in one turn, the later text is the one sent.
func TestTheLatestMarkInATurnWins(t *testing.T) {
	dir := stateDir(t)
	mark(t, dir, "e1", "first", 150)
	mark(t, dir, "e1", "second", 160)
	if text, ok := take(t, dir, "e1", 100, 200); !ok || text != "second" {
		t.Errorf("got %q %v", text, ok)
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
