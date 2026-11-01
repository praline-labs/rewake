package mail

import (
	"os"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// The unread work beside what is owed counts tasks and questions, any run's,
// and not one already owed: a read whose last step failed leaves its task in
// unread/ and owed at once, and it is counted once, as owed.
func TestTheOwedHintCountsUnreadWorkNotOwed(t *testing.T) {
	owed := task("owed and still in unread/")
	dir, box := readLab(t, owed)
	done := state.DonePath(dir, "api")
	if err := os.MkdirAll(done, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(done, 0o500); err != nil {
		t.Fatal(err)
	}
	_, err := box.Read(t.Context(), ReadRequest{Reports: true}, showNothing)
	if err := os.Chmod(done, 0o700); err != nil {
		t.Fatal(err)
	}
	if stepOf(err) != StepMark {
		t.Fatalf("the read whose move failed answered %v", err)
	}

	question := task("which port?")
	question.Kind = Question
	earlier := task("for an earlier run")
	earlier.ToEpoch = "4.4"
	note := task("a note")
	note.Kind = Note
	report := task("a report")
	report.Kind = Finished
	unread(t, dir, question, earlier, note, report)

	summary, err := box.Owed()
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Owed) != 1 || summary.Owed[0].ID != owed.ID || !summary.Owed[0].Kept {
		t.Fatalf("owed %+v, want the task read", summary.Owed)
	}
	if summary.Unread != 2 {
		t.Fatalf("%d unread counted, want the question and the earlier run's task", summary.Unread)
	}
}

// A look that fails answers nothing rather than part of it, and names which.
func TestAnOwedLookThatFailsAnswersNothing(t *testing.T) {
	dir, box := readLab(t, task("unseen"))
	path := state.UnreadPath(dir, "api")
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	summary, err := box.Owed()
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if stepOf(err) != StepUnread || summary.Owed != nil || summary.Unread != 0 {
		t.Fatalf("with unread/ closed: %+v, %v", summary, err)
	}

	if _, err := box.Read(t.Context(), ReadRequest{Reports: true}, showNothing); err != nil {
		t.Fatal(err)
	}
	waits := state.AwaitingPath(dir, "api")
	if err := os.Chmod(waits, 0); err != nil {
		t.Fatal(err)
	}
	summary, err = box.Owed()
	if err := os.Chmod(waits, 0o700); err != nil {
		t.Fatal(err)
	}
	if stepOf(err) != StepOwed || summary.Owed != nil {
		t.Fatalf("with the waits closed: %+v, %v", summary, err)
	}
}
