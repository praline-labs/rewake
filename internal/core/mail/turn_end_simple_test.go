package mail

import (
	"errors"
	"os"
	"path/filepath"
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
// once its run is gone the sender reads the task as lost.
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
	if err != nil || len(awaited) != 1 || awaited[0].ID != lab.task.ID || !awaited[0].Gone() {
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

// readLongAgo has web read the report and a day pass, then web's sweep run:
// it answers whether the letter is still kept.
func (lab endLab) readLongAgo(t *testing.T, report Message) bool {
	t.Helper()
	if err := move(report.ID, state.InboxPath(lab.dir, "web"), state.DonePath(lab.dir, "web")); err != nil {
		t.Fatal(err)
	}
	done := filepath.Join(state.DonePath(lab.dir, "web"), report.ID+".json")
	old := time.Now().Add(-keepFinished - time.Hour)
	if err := os.Chtimes(done, old, old); err != nil {
		t.Fatal(err)
	}
	sweepAsLiveRun(t, lab.dir, "web", lab.web.Epoch())
	_, err := os.Stat(done)
	return err == nil
}

// The sweep keeps a report its writer's wait still names, read a day ago: the
// end retried after that finds the letter and writes no second one, and once
// the wait is cleared the next sweep removes it. (Reworked from
// TestAJournalDoesNotRepublishASweptReport.)
func TestASweepKeepsAReportWhileItsWaitStands(t *testing.T) {
	lab := newEndLab(t)
	end := lab.end(t, "turn-1")
	failFirst(t, state.OpRemove, state.AwaitingPath(lab.dir, "api"))
	if err := lab.complete(t, end); err == nil {
		t.Fatal("the clearing did not fail")
	}
	state.Fault = nil
	report := lab.finished(t)[0]
	if !lab.readLongAgo(t, report) {
		t.Fatal("the sweep removed a report whose wait stands")
	}
	if err := lab.complete(t, end); err != nil {
		t.Fatal(err)
	}
	if got := lab.finished(t); len(got) != 0 || lab.owed() != 0 {
		t.Fatalf("after the retry web has %d new reports, api owes %d", len(got), lab.owed())
	}
	sweepAsLiveRun(t, lab.dir, "web", lab.web.Epoch())
	if _, err := os.Stat(filepath.Join(state.DonePath(lab.dir, "web"), report.ID+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the sweep kept a report whose wait is cleared: %v", err)
	}
}

// An end that wrote its report and cleared its wait, and could not record
// itself done, retried after the report was read and swept, writes nothing
// again: no wait names the task any more. (Reworked from
// TestAReportWhoseEntryFailedIsNotRepublishedAfterTheSweep.)
func TestAnEndNotRecordedDoneIsNotRepublishedAfterTheSweep(t *testing.T) {
	lab := newEndLab(t)
	end := lab.end(t, "turn-1")
	failFirst(t, state.OpWrite, JournalPath(lab.dir, "api"))
	if err := lab.complete(t, end); err == nil {
		t.Fatal("the record of the end did not fail")
	}
	state.Fault = nil
	if lab.readLongAgo(t, lab.finished(t)[0]) {
		t.Fatal("the sweep kept a report whose wait is cleared")
	}
	if err := lab.complete(t, end); err != nil {
		t.Fatal(err)
	}
	if got := lab.finished(t); len(got) != 0 || lab.owed() != 0 {
		t.Fatalf("after the retry web has %d new reports, api owes %d", len(got), lab.owed())
	}
}
