package mail

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/boottime"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// endLab is api's live run, which has read one task from web delivered into a
// conversation, and the end of its turn as a harness's boundary hands it over.
type endLab struct {
	twoSessionLab
	self registry.Session
	task Message
}

func newEndLab(t *testing.T) endLab {
	t.Helper()
	lab := endLab{twoSessionLab: newTwoSessionLab(t)}
	lab.self = lab.session(t, "api")
	lab.task = lab.read(t, "do it")
	return lab
}

// read puts a task from web in api's mailbox and has api's run read it.
func (lab endLab) read(t *testing.T, text string) Message {
	t.Helper()
	task := Message{ID: NewID(), From: "web", FromEpoch: lab.web.Epoch(), To: "api", ToEpoch: lab.self.Epoch(), Kind: Task, Text: text, CreatedAt: time.Now()}
	if err := Put(lab.dir, task); err != nil {
		t.Fatal(err)
	}
	readIn(t, lab.dir, lab.self.Epoch(), "thread-1", task)
	return task
}

// end is the end of turn id, its scope everything read so far.
func (lab endLab) end(t *testing.T, id string) TurnEnd {
	t.Helper()
	clock, err := OpenReadClock(t.Context(), lab.dir, "api", lab.self.Epoch())
	if err != nil {
		t.Fatal(err)
	}
	defer clock.Close()
	return TurnEnd{ID: id, Text: "done", Started: boottime.Now() - 1000, Ended: boottime.Now(), Boundary: clock.Snapshot()}
}

func (lab endLab) complete(t *testing.T, end TurnEnd) error {
	t.Helper()
	return CompleteTurn(t.Context(), lab.dir, lab.self, end, "thread-1", TurnEndOptions{})
}

// finished is what web holds of api's finished reports.
func (lab endLab) finished(t *testing.T) []Message {
	t.Helper()
	var reports []Message
	for _, letter := range turnReports(t, lab.dir) {
		if KindOf(letter) == Finished {
			reports = append(reports, letter)
		}
	}
	return reports
}

func (lab endLab) owed() int { return len(Waiters(lab.dir, "api", lab.self.Epoch())) }

// failFirst fails the first operation op on a path that contains part.
func failFirst(t *testing.T, op, part string) *bool {
	t.Helper()
	failed := false
	state.Fault = func(got, path string) error {
		if !failed && got == op && strings.Contains(path, part) {
			failed = true
			return os.ErrPermission
		}
		return nil
	}
	t.Cleanup(func() { state.Fault = nil })
	return &failed
}

// A turn end answers the task it read with one report and clears the wait.
func TestATurnEndReportsOnceAndClearsTheWait(t *testing.T) {
	lab := newEndLab(t)
	if err := lab.complete(t, lab.end(t, "turn-1")); err != nil {
		t.Fatal(err)
	}
	if got := lab.finished(t); len(got) != 1 || got[0].InReplyTo[0] != lab.task.ID || lab.owed() != 0 {
		t.Fatalf("web holds %+v, api owes %d", got, lab.owed())
	}
}

// An end whose report was written and whose wait could not be cleared, sent
// again, clears the wait and writes no second report.
func TestAnEndRetriedAfterTheClearingFailedReportsOnce(t *testing.T) {
	lab := newEndLab(t)
	end := lab.end(t, "turn-1")
	failed := failFirst(t, state.OpRemove, state.AwaitingPath(lab.dir, "api"))
	if err := lab.complete(t, end); err == nil || !*failed {
		t.Fatalf("the clearing did not fail: %v %v", err, *failed)
	}
	if got := lab.finished(t); len(got) != 1 || lab.owed() != 1 {
		t.Fatalf("after the failure web holds %d reports, api owes %d", len(got), lab.owed())
	}
	state.Fault = nil
	if err := lab.complete(t, end); err != nil {
		t.Fatal(err)
	}
	if got := lab.finished(t); len(got) != 1 || lab.owed() != 0 {
		t.Fatalf("after the retry web holds %d reports, api owes %d", len(got), lab.owed())
	}
}

// An end that dies before its report is written leaves the wait standing, and
// once its run is gone the sender reads the task as one a resume may take over.
func TestAnEndThatFailedBeforeTheReportLeavesTheWaitStanding(t *testing.T) {
	lab := newEndLab(t)
	failed := failFirst(t, state.OpWrite, state.InboxPath(lab.dir, "web"))
	if err := lab.complete(t, lab.end(t, "turn-1")); err == nil || !*failed {
		t.Fatalf("the report did not fail: %v %v", err, *failed)
	}
	state.Fault = nil
	if got := lab.finished(t); len(got) != 0 || lab.owed() != 1 {
		t.Fatalf("web holds %d reports, api owes %d", len(got), lab.owed())
	}
	awaited, err := Awaited(lab.dir, "web", lab.web.Epoch(), func(string, string) RecipientRun { return RunEnded })
	if err != nil || len(awaited) != 1 || awaited[0].ID != lab.task.ID || !awaited[0].Resumable || awaited[0].Gone() {
		t.Fatalf("web reads %+v: %v", awaited, err)
	}
}

// An end sent again once it is done does nothing: no second report, and a
// task read since stays owed for the next end.
func TestAnEndSentAgainOnceDoneDoesNothing(t *testing.T) {
	lab := newEndLab(t)
	end := lab.end(t, "turn-1")
	if err := lab.complete(t, end); err != nil {
		t.Fatal(err)
	}
	later := lab.read(t, "and this")
	if err := lab.complete(t, end); err != nil {
		t.Fatal(err)
	}
	waiters := Waiters(lab.dir, "api", lab.self.Epoch())
	if got := lab.finished(t); len(got) != 1 || len(waiters) != 1 || waiters[0].Messages[0] != later.ID {
		t.Fatalf("web holds %d reports, api owes %+v", len(got), waiters)
	}
}
