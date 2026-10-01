package inbox

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// A conversion that died after its journal was saved and before it removed
// the receipts left receipts it holds already: the next barrier removes them
// and goes on, weighing each as the journal records it
// (docs/turn-end-recovery.md#reconciliation, step 2).
func TestAConversionThatDiedBeforeRemovingItsReceiptsGoesOn(t *testing.T) {
	lab := newConversionLab(t)
	lab.owe(t, "task")
	report := lab.report(Finished, "task")
	path := lab.receipt(t, true, false, report)
	receipts, err := earlierReceipts(lab.dir, "api")
	if err != nil {
		t.Fatal(err)
	}
	if err := (&conversionJournal{Receipts: receipts}).save(lab.dir, "api"); err != nil {
		t.Fatal(err)
	}
	if err := lab.reconcile(t); err != nil {
		t.Fatalf("the conversion's own receipt stopped it: %v", err)
	}
	if lab.owes(t, "task") || lab.copies(t, report.ID) != 0 {
		t.Fatalf("a done report: owed %v, copies %d", lab.owes(t, "task"), lab.copies(t, report.ID))
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the receipt stayed: %v", err)
	}
}

// A receipt the conversion journal holds under its file but that reads
// otherwise now was written again since, by a writer of the earlier build
// still running: that is the signal the boundary was crossed, and it stops.
func TestAReceiptChangedAfterItsConversionStops(t *testing.T) {
	lab := newConversionLab(t)
	lab.owe(t, "task")
	path := lab.receipt(t, false, false, lab.report(Finished, "task"))
	receipts, err := earlierReceipts(lab.dir, "api")
	if err != nil {
		t.Fatal(err)
	}
	if err := (&conversionJournal{Receipts: receipts}).save(lab.dir, "api"); err != nil {
		t.Fatal(err)
	}
	receipts[0].Done = true
	raw, err := json.Marshal(receipts[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if lab.reconcile(t) == nil || MailboxStopped(lab.dir, "api") == nil {
		t.Fatal("a receipt changed after the conversion went through")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the changed receipt was removed: %v", err)
	}
}

// The barrier reads every record before any effect: an unreadable journal
// beside one ready to publish stops both, and a later look that reads it
// lets both go (8-stop).
func TestAnUnreadableJournalStopsEveryEffect(t *testing.T) {
	lab := newConversionLab(t)
	report := lab.report(Finished, "task")
	if err := WriteJournal(lab.dir, "api", "a", TurnJournal{Epoch: earlierRun, Reports: []Message{report}}); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(JournalPath(lab.dir, "api"), "z")
	if err := os.WriteFile(broken, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if lab.reconcile(t) == nil {
		t.Fatal("an unreadable journal went through")
	}
	if lab.copies(t, report.ID) != 0 {
		t.Fatal("a journal published beside an unreadable one")
	}
	if err := os.Remove(broken); err != nil {
		t.Fatal(err)
	}
	if err := lab.reconcile(t); err != nil {
		t.Fatal(err)
	}
	if lab.copies(t, report.ID) != 1 {
		t.Fatalf("published %d times once readable", lab.copies(t, report.ID))
	}
}

// Every call that would change the mailbox asks the same question the barrier
// does: a journal or a wait record that cannot be read is a stop whichever call
// finds it. The stop is recorded, so it stands once its cause is gone, for
// every call but the barrier, which reads again and lifts it.
func TestAnUnreadableRecordStopsTheMailbox(t *testing.T) {
	for _, record := range []string{"journal", "wait"} {
		t.Run(record, func(t *testing.T) {
			lab := newConversionLab(t)
			lab.owe(t, "task")
			path := filepath.Join(JournalPath(lab.dir, "api"), "broken")
			if record == "wait" {
				path = filepath.Join(state.AwaitingPath(lab.dir, "api"), earlierRun, "web")
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			// More fields than any wait record carries: it parses as none.
			if err := os.WriteFile(path, []byte("{ 1 2 3 4 5 6"), 0o600); err != nil {
				t.Fatal(err)
			}
			if MailboxStopped(lab.dir, "api") == nil {
				t.Fatalf("an unreadable %s did not stop the mailbox", record)
			}
			if _, err := os.Stat(stopPath(lab.dir, "api")); err != nil {
				t.Fatalf("the stop was not recorded: %v", err)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			// The reading found it, and the same reading finds it gone.
			if err := MailboxStopped(lab.dir, "api"); err != nil {
				t.Fatalf("a stop whose cause is gone: %v", err)
			}
			if _, err := os.Stat(stopPath(lab.dir, "api")); !os.IsNotExist(err) {
				t.Fatalf("the stop outlived a reading that found nothing: %v", err)
			}
		})
	}
}

// A successor recorded as the recipient by an attempt that died before it
// published proves nothing about its state now: once it has ended the report
// is moot and main is told, and nothing is published to it.
func TestAChosenSuccessorThatEndedBeforePublicationIsMoot(t *testing.T) {
	lab := newConversionLab(t)
	report := lab.holdReport(t)
	lab.chooseSuccessor(t, report, lab.bind(t, 4194001, 9))
	path := filepath.Join(JournalPath(lab.dir, "web"), "end")
	for range 2 {
		lab.barrier(t)
	}
	if len(lab.taken(t, report)) != 0 || lab.notes(t) != 1 {
		t.Fatalf("taken %d, notes %d", len(lab.taken(t, report)), lab.notes(t))
	}
	if err := withLock(lab.dir, "web", func() error { return Reconcile(context.Background(), lab.dir, "web") }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the journal of a moot report is not done: %v", err)
	}
}

// A wait of a run of this build without its place on the read clock was
// written by a writer of the earlier build after the successor was bound:
// this build writes none such. It stops the mailbox, as the other signals of
// a crossed boundary do, before an end heard once answers it unscoped.
func TestAWaitWithoutItsPlaceOnTheClockStops(t *testing.T) {
	lab := newConversionLab(t)
	run := registry.RunEpoch(4194002, 11, lab.web.Boot)
	if err := markAwaiting(lab.dir, "api", run, "web", lab.web.Epoch(), "task"); err != nil {
		t.Fatal(err)
	}
	if lab.reconcile(t) == nil || MailboxStopped(lab.dir, "api") == nil {
		t.Fatal("a wait without its place on the clock went through")
	}
	if err := markAwaiting(lab.dir, "api", earlierRun, "web", lab.web.Epoch(), "old"); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(state.AwaitingPath(lab.dir, "api"), run)); err != nil {
		t.Fatal(err)
	}
	if err := lab.reconcile(t); err != nil {
		t.Fatalf("an earlier-build run's wait, which never had a place, stopped it: %v", err)
	}
}
