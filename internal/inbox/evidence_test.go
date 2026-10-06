package inbox

import (
	"os"
	"path/filepath"
	"testing"
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
			lab := newTwoSessionLab(t)
			first, last := lab.report(Finished, "first"), lab.report(Finished, "last")
			version := "v1"
			journal := TurnJournal{Epoch: lab.run, Op: "end", Ended: 100, Reports: []Message{first, last}}
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
			if len(stopsOnRecord(t, lab.dir, "api")) == 0 {
				t.Fatal("the stop was not recorded")
			}
			if MailboxStopped(lab.dir, "api") == nil {
				t.Fatalf("a later call went on past the unknown %s", cause)
			}
		})
	}
}
