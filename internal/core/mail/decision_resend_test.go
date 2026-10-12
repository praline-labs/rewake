package mail

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// doneResendRaw is the done form of d's resend with summary as its bytes.
func doneResendRaw(t *testing.T, d Decision, summary string) string {
	t.Helper()
	held, err := d.resendJournal()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{"Epoch": held.Epoch, "Op": held.Op, "Done": true, "Summary": json.RawMessage(summary)})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// Bytes under a resend's name are the resend only when they are its envelope,
// open or done, and the resend its decision holds. A record of another kind,
// a journal of another run, operation or report, an open one that keeps a
// summary or one renamed done that does not say so is an unknown: the view
// names it, the barrier stops on it every time, nothing is published on it,
// the original journal's tail does not run, and the bytes stay as they are.
func TestDecisionResendOfAnotherKindStaysUnknown(t *testing.T) {
	cases := map[string]struct {
		done bool
		raw  func(t *testing.T, d Decision) string
	}{
		"a record of another kind":       {false, func(*testing.T, Decision) string { return `{"answer":"a different record"}` }},
		"a record of another kind, done": {true, func(*testing.T, Decision) string { return `{"answer":"a different record"}` }},
		"another report": {false, func(_ *testing.T, d Decision) string {
			held, _ := d.resendJournal()
			held.Reports[0].Text = "another text"
			raw, _ := json.Marshal(held)
			return string(raw)
		}},
		"another run": {true, func(_ *testing.T, d Decision) string {
			held, _ := d.resendJournal()
			raw, _ := json.Marshal(TurnJournal{Epoch: "another-run", Op: held.Op, Done: true})
			return string(raw)
		}},
		"another operation": {false, func(_ *testing.T, d Decision) string {
			held, _ := d.resendJournal()
			held.Op = NewID()
			raw, _ := json.Marshal(held)
			return string(raw)
		}},
		"another operation, done": {true, func(_ *testing.T, d Decision) string {
			held, _ := d.resendJournal()
			raw, _ := json.Marshal(TurnJournal{Epoch: held.Epoch, Op: NewID(), Done: true})
			return string(raw)
		}},
		"a summary while open": {false, func(_ *testing.T, d Decision) string {
			held, _ := d.resendJournal()
			held.Summary = &ResendSummary{Report: held.Reports[0].ID, Disposition: DispositionMoot, Decision: d.ID}
			raw, _ := json.Marshal(held)
			return string(raw)
		}},
		"renamed done and open": {true, func(_ *testing.T, d Decision) string {
			held, _ := d.resendJournal()
			raw, _ := json.Marshal(held)
			return string(raw)
		}},
		"a summary of another report": {true, func(t *testing.T, d Decision) string {
			return doneResendRaw(t, d, `{"report":"`+NewID()+`","disposition":"moot","decision":"`+d.ID+`"}`)
		}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			lab := newDecisionLab(t)
			d := lab.decision(t, ChoiceNotArrived)
			writeDecision(t, lab.dir, d)
			path := filepath.Join(JournalPath(lab.dir, "api"), ResendJournal(d.ID))
			if c.done {
				path += doneSuffix
			}
			raw := c.raw(t, d)
			writeRaw(t, path, raw)
			views, err := Decisions(lab.dir, "api")
			if err != nil || len(views) != 1 || views[0].Resend.Err == nil || views[0].Resend.Done {
				t.Fatalf("the view took it for the resend: %v %+v", err, views)
			}
			for range 2 {
				if err := finishAll(t, lab.dir); err == nil {
					t.Fatal("the barrier went past it")
				}
			}
			if after, err := os.ReadFile(path); err != nil || string(after) != raw {
				t.Fatalf("it was not kept: %q %v", after, err)
			}
			if fileExists(filepath.Join(JournalPath(lab.dir, "api"), "end"+doneSuffix)) || live(lab.dir).installed("api") {
				t.Fatal("the original journal ran, or the installation took it for done")
			}
			held, _ := d.resendJournal()
			if lettersOf(t, lab.dir, "web")[held.Reports[0].ID] != 0 {
				t.Fatal("the resend's report was published on it")
			}
		})
	}
}

// A done resend's summary is its outcome and nothing more: one that does not
// read, in either done form, leaves the resend done — completed, outcome not
// kept — and stops nothing. The view says so, never a disposition, and the
// barrier goes on with the original journal.
func TestDecisionResendSummaryThatDoesNotReadKeepsItsCompletion(t *testing.T) {
	for form, renamed := range map[string]bool{"done in place": false, "renamed done": true} {
		for name, summary := range map[string]func(report, decision string) string{
			"no disposition": func(report, decision string) string {
				return `{"report":"` + report + `","disposition":"lost","decision":"` + decision + `"}`
			},
			"no summary": func(string, string) string { return `"a word"` },
		} {
			t.Run(form+", "+name, func(t *testing.T) {
				lab := newDecisionLab(t)
				d := lab.decision(t, ChoiceNotArrived)
				writeDecision(t, lab.dir, d)
				held, _ := d.resendJournal()
				path := filepath.Join(JournalPath(lab.dir, "api"), ResendJournal(d.ID))
				if renamed {
					path += doneSuffix
				}
				writeRaw(t, path, doneResendRaw(t, d, summary(held.Reports[0].ID, d.ID)))
				views, err := Decisions(lab.dir, "api")
				if err != nil || len(views) != 1 {
					t.Fatalf("the views: %v %+v", err, views)
				}
				if resend := views[0].Resend; !resend.Installed || !resend.Done || resend.Summary != nil || resend.SummaryErr == nil || resend.Err != nil {
					t.Fatalf("the view says %+v", resend)
				}
				if err := finishAll(t, lab.dir); err != nil {
					t.Fatalf("the barrier stopped on a summary: %v", err)
				}
				if !fileExists(filepath.Join(JournalPath(lab.dir, "api"), "end"+doneSuffix)) || lettersOf(t, lab.dir, "web")[held.Reports[0].ID] != 0 {
					t.Fatal("the original journal did not finish, or the resend was published again")
				}
			})
		}
	}
}
