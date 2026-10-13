package mail

import (
	"context"
	"path/filepath"
	"testing"
)

// A decision disposes of a report of the journal's own run alone: a report
// the journal carries with another sender run, or none, is no subject, found
// or decided, for either choice, and nothing is written.
func TestDecideHoldsTheReportToTheJournalsRun(t *testing.T) {
	for _, run := range []string{"another-run", ""} {
		for _, choice := range []string{ChoiceArrived, ChoiceNotArrived} {
			t.Run(choice+" from run "+run, func(t *testing.T) {
				lab := newUnknownLab(t)
				req := lab.ask(t, choice, DecidedBy{Kind: ByPerson})
				path := filepath.Join(JournalPath(lab.dir, "api"), "end")
				j, err := readJournalFile(path)
				if err != nil {
					t.Fatal(err)
				}
				j.Reports[0].FromEpoch = run
				if err := WriteJournal(lab.dir, "api", "end", j); err != nil {
					t.Fatal(err)
				}
				if subject, err := FindSubject(lab.dir, "api", lab.report.ID); outcomeOf(err) != DecisionNoSubject {
					t.Fatalf("found %+v, %v; want no subject", subject, err)
				}
				before := wholeTree(t, lab.dir)
				answer, err := Decide(context.Background(), lab.dir, req)
				if outcomeOf(err) != DecisionNoSubject {
					t.Fatalf("answered %+v, %v; want no subject", answer, err)
				}
				if after := wholeTree(t, lab.dir); !sameTree(before, after) {
					t.Fatalf("a refused decision wrote: %v", treeDiff(before, after))
				}
			})
		}
	}
}
