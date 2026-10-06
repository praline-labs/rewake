package inbox

import (
	"testing"

	"github.com/praline-labs/rewake/internal/registry"
)

// endTurn writes api's journal of one turn end: report, and clearing what
// api's run owes web for tasks.
func (l twoSessionLab) endTurn(t *testing.T, report Message, tasks ...string) {
	t.Helper()
	waiters, err := ReadWaiters(l.dir, "api", l.run)
	if err != nil || len(waiters) != 1 {
		t.Fatalf("waiters %v: %v", waiters, err)
	}
	answered := waiters[0]
	answered.Messages, answered.ReadSequences, answered.ReadAt = tasks, nil, nil
	if err := WriteJournal(l.dir, "api", "end", TurnJournal{Epoch: l.run, Op: "end", Ended: 100, Reports: []Message{report}, Clear: []Waiter{answered}}); err != nil {
		t.Fatal(err)
	}
}

// A report whose recipient is a run that was replaced, or whose name is gone,
// can reach nobody: nothing proves it either way, and still it neither stops
// the mailbox nor goes out (docs/turn-end-recovery.md#reconciliation).
func TestAReportForAnEndedRunIsMoot(t *testing.T) {
	for _, recipient := range []string{"replaced", "gone"} {
		t.Run(recipient, func(t *testing.T) {
			lab := newTwoSessionLab(t)
			lab.owe(t, "t1")
			report := lab.report(Finished, "t1")
			report.ToEpoch = registry.RunEpoch(lab.web.ServicePID, lab.web.ServiceStart+1, lab.web.Boot)
			if recipient == "gone" {
				report.To = "left"
			}
			lab.endTurn(t, report, "t1")
			if err := lab.reconcile(t); err != nil {
				t.Fatalf("a report nobody can receive stopped the mailbox: %v", err)
			}
			if MailboxStopped(lab.dir, "api") != nil || lab.copies(t, report.ID) != 0 {
				t.Fatalf("stopped=%v copies=%d", MailboxStopped(lab.dir, "api"), lab.copies(t, report.ID))
			}
		})
	}
}

// What a moot closing report answered is cleared with its journal, as what a
// published one answered is: no report will answer it, and a wait left owed
// would be taken over by adoption for a run nobody can report to
// (docs/turn-end-recovery.md#reconciliation).
func TestAMootReportClearsWhatItAnswered(t *testing.T) {
	lab := newTwoSessionLab(t)
	lab.owe(t, "task", "other")
	lab.endTurn(t, lab.report(Finished, "task"), "task")
	if _, err := registry.RemoveOwned(lab.dir, "web", lab.web.Epoch()); err != nil {
		t.Fatal(err)
	}
	if err := lab.reconcile(t); err != nil {
		t.Fatal(err)
	}
	if lab.owes(t, "task") || !lab.owes(t, "other") {
		t.Fatalf("owed after the journal: task %v, other %v", lab.owes(t, "task"), lab.owes(t, "other"))
	}
}
