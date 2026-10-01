package inbox

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/praline-labs/rewake/internal/state"
)

func writeRaw(t *testing.T, path, raw string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
}

// What every remaining effect decides by is read before the first of them
// (8-stop): an unknown publication mark of the last report, or a kept answer
// or interim record that does not read, stops the barrier before the first
// report is published, and the stop is on record for the next call.
func TestTheEvidenceOfEveryEffectIsReadBeforeTheFirst(t *testing.T) {
	for _, cause := range []string{"publication", "kept", "interim"} {
		t.Run(cause, func(t *testing.T) {
			lab := newConversionLab(t)
			first, last := lab.report(Finished, "first"), lab.report(Finished, "last")
			version := "v1"
			journal := TurnJournal{Epoch: earlierRun, Op: "end", Ended: 100, Reports: []Message{first, last}}
			switch cause {
			case "publication":
				path, _ := oncePath(lab.dir, last.To, last.ToEpoch, last.ID)
				writeRaw(t, path, "neither")
			case "kept":
				journal.Kept = &version
				writeRaw(t, keptPath(lab.dir, "api"), "{")
			case "interim":
				journal.Settles = true
				writeRaw(t, interimPath(lab.dir, "api"), "{")
			}
			if err := WriteJournal(lab.dir, "api", "end", journal); err != nil {
				t.Fatal(err)
			}
			if err := lab.reconcile(t); err == nil {
				t.Fatalf("the barrier went on past an unknown %s", cause)
			}
			if copies := lab.copies(t, first.ID); copies != 0 {
				t.Fatalf("the first report went out before the unknown %s was found: %d", cause, copies)
			}
			if _, err := os.Stat(stopPath(lab.dir, "api")); err != nil {
				t.Fatalf("the stop was not recorded: %v", err)
			}
			if MailboxStopped(lab.dir, "api") == nil {
				t.Fatalf("a later call went on past the unknown %s", cause)
			}
		})
	}
}

// The note to main that a held report is moot is owed with the decision:
// one that could not be published is sent by the next barrier, and once.
func TestAHeldMootNoteIsOwedUntilSent(t *testing.T) {
	lab := newConversionLab(t)
	report := lab.holdReport(t)
	lab.chooseSuccessor(t, report, lab.bind(t, 4194001, 9))
	// Main's mailbox reads but takes no write: the plan finds nothing
	// unknown, and the note fails only once the decision is recorded.
	mailbox := state.InboxPath(lab.dir, "lead")
	if err := os.MkdirAll(mailbox, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(mailbox, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(mailbox, 0o700) })
	if err := withLock(lab.dir, "web", func() error { return Reconcile(context.Background(), lab.dir, "web") }); err == nil {
		t.Fatal("the note went out through a closed mailbox")
	}
	journal, err := readJournalFile(filepath.Join(JournalPath(lab.dir, "web"), "end"))
	if err != nil || !slices.Contains(journal.Moot, report.ID) || !slices.Contains(journal.Notices, report.ID) {
		t.Fatalf("the moot decision without its note owed: %+v %v", journal, err)
	}
	if err := os.Chmod(mailbox, 0o700); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		lab.barrier(t)
	}
	if notes := lab.notes(t); notes != 1 {
		t.Fatalf("notes to main: %d", notes)
	}
	if _, ok, err := heldMootNote(lab.dir, "web", report.ID); err != nil || !ok {
		t.Fatalf("the one note is not the one about %s: %v", report.ID, err)
	}
}
