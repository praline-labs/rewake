package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/mail"
)

// A report's operation is named once: a turn journal is named by its
// operation, and the two ids printed side by side read as two operations.
func TestTheOperationIsNamedOnce(t *testing.T) {
	for _, c := range []struct {
		op   operationModel
		want string
	}{
		{operationModel{Journal: "end"}, "operation end"},
		{operationModel{Journal: "1790370984617481100-00000000000b", Op: "1790370984617481100-00000000000b"}, "operation 1790370984617481100-00000000000b"},
		{operationModel{Journal: "decided-d1", Op: "o1"}, "operation o1 in journal decided-d1"},
	} {
		if got := operationText(c.op); got != c.want {
			t.Errorf("%+v reads %q, want %q", c.op, got, c.want)
		}
	}
}

// What a report decided "arrived" answers is listed from the records that
// keep it — the closure its journal's tail wrote, or the journal itself —
// and, with both gone, is said to be unknown rather than listed as none.
func TestTheListSaysWhatAnArrivedReportAnswers(t *testing.T) {
	lab := newDecideLab(t)
	(&route{}).install(t)
	if code, out, errOut := run("decide", "api", lab.report.ID, "--arrived"); code != ExitOK {
		t.Fatalf("decide: exit %d\n%s%s", code, out, errOut)
	}
	d := lab.decision(t)
	listed := func() subjectModel {
		t.Helper()
		var listing decideListModel
		_, out, _ := run("decide", "api", "--json")
		if err := json.Unmarshal([]byte(out), &listing); err != nil || len(listing.Subjects) != 1 {
			t.Fatalf("%v:\n%s", err, out)
		}
		return listing.Subjects[0]
	}
	if s := listed(); s.AnswersUnknown || len(s.Answers) != 1 || s.Answers[0] != questionID {
		t.Fatalf("closed by %s, the report answers %v (unknown %v)", d.ID, s.Answers, s.AnswersUnknown)
	}
	if _, out, _ := run("decide", "api"); !strings.Contains(out, "answering "+questionID) {
		t.Fatalf("the list:\n%s", out)
	}
	// The closure and the journal gone, nothing keeps what it answered.
	journals := mail.JournalPath(lab.dir, "api")
	for _, path := range []string{mail.DecisionPath(lab.dir, "api", lab.report.ID) + ".closed", filepath.Join(journals, "end"), filepath.Join(journals, "end.done")} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	if s := listed(); !s.AnswersUnknown || len(s.Answers) != 0 {
		t.Fatalf("with nothing kept, the report answers %v (unknown %v)", s.Answers, s.AnswersUnknown)
	}
	if _, out, _ := run("decide", "api"); !strings.Contains(out, "what it answers no longer kept") {
		t.Fatalf("the list:\n%s", out)
	}
}
