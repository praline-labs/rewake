package mail

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A told record says one note was told to one run. One that names another
// note, another name or another run says nothing of the decision's note: the
// installation, its evidence and the view take it for an unknown alike —
// never told, so the note is not suppressed, and never owed, so it is not
// written over — and the barrier stops on it.
func TestDecisionToldRecordsOfAnotherNoteTellNothing(t *testing.T) {
	for _, field := range []string{"note", "name", "run"} {
		t.Run(field, func(t *testing.T) {
			lab := newDecisionLab(t)
			d := lab.decision(t, ChoiceArrived)
			writeDecision(t, lab.dir, d)
			note := d.Notes[0].Note
			told := ToldRecord{Name: note.To, Run: note.ToEpoch, Note: note.ID, Answer: ToldWritten, At: time.Now()}
			switch field {
			case "note":
				told.Note = NewID()
			case "name":
				told.Name = "other"
			case "run":
				told.Run = "other-run"
			}
			path := toldPath(lab.dir, "api", lab.report.ID, ToldRecipient)
			writeRecord(t, path, told)
			before, _ := os.ReadFile(path)
			for range 2 {
				if err := finishAll(t, lab.dir); err == nil {
					t.Fatal("the barrier took a told record of another note for this one")
				}
			}
			if lettersOf(t, lab.dir, "web")[note.ID] != 0 || fileExists(filepath.Join(JournalPath(lab.dir, "api"), "end"+doneSuffix)) {
				t.Fatal("the note was told, or the journal ran, beside a told record of another note")
			}
			if after, _ := os.ReadFile(path); string(after) != string(before) {
				t.Fatal("the told record of another note was written over")
			}
			if live(lab.dir).installed("api") {
				t.Fatal("the installation's evidence took it for told")
			}
			views, err := Decisions(lab.dir, "api")
			if err != nil || len(views) != 1 {
				t.Fatalf("the views: %v %+v", err, views)
			}
			if view := views[0].Told[ToldRecipient]; view.Err == nil || view.Record != nil {
				t.Fatalf("the view says %+v", view)
			}
		})
	}
}
