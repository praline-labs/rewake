package inbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/state"
)

// The barrier reads every record before any effect: an unreadable journal
// beside one ready to publish stops both, and a later look that reads it
// lets both go (8-stop).
func TestAnUnreadableJournalStopsEveryEffect(t *testing.T) {
	lab := newTwoSessionLab(t)
	report := lab.report(Finished, "task")
	if err := WriteJournal(lab.dir, "api", "a", TurnJournal{Epoch: lab.run, Reports: []Message{report}}); err != nil {
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
			lab := newTwoSessionLab(t)
			lab.owe(t, "task")
			path := filepath.Join(JournalPath(lab.dir, "api"), "broken")
			if record == "wait" {
				path = filepath.Join(state.AwaitingPath(lab.dir, "api"), lab.run, "web")
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
