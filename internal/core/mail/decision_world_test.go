package mail

import (
	"fmt"
	"testing"

	"github.com/praline-labs/rewake/internal/core/mail/decisionmodel"
)

// The copies a chain of decisions leaves follow the world's truth, never the
// choices: each copy's outcome is made unknown by a mark that says nothing,
// it lands or not as the world says, and the operator decides it either way.
// The model (decisionmodel.Expect) predicts from those dimensions alone how
// many copies exist and how many landed; the letters r holds, nothing read or
// swept, are the landings, and R is never written again. One generation and
// two; the chain of forty is the manifest's (part 3).
func TestDecisionCopiesFollowWorldTruth(t *testing.T) {
	choices := []string{ChoiceArrived, ChoiceNotArrived}
	truths := []bool{false, true}
	for _, x := range truths {
		for _, first := range choices {
			chain := []decisionmodel.Copy{{Landed: x, Decided: first}}
			worldOf(t, chain)
			if first != ChoiceNotArrived {
				continue
			}
			for _, y := range truths {
				for _, second := range choices {
					chain := []decisionmodel.Copy{{Landed: x, Decided: first}, {Landed: y, Decided: second}}
					worldOf(t, chain)
				}
			}
		}
	}
}

// worldOf runs one chain: every copy but a last one nobody decides is made
// unknown, lands as its truth says, and is decided; a copy past the last
// decision is published as any report is.
func worldOf(t *testing.T, chain []decisionmodel.Copy) {
	name := ""
	for _, c := range chain {
		name += fmt.Sprintf("%v-%s/", c.Landed, c.Decided)
	}
	t.Run(name, func(t *testing.T) {
		dir, epoch, report := journalLab(t)
		journal, parent := "end", ""
		var copies []Message
		for _, c := range chain {
			copies = append(copies, report)
			damageMark(t, dir, report)
			if c.Landed {
				if err := live(dir).put(report); err != nil {
					t.Fatal(err)
				}
			}
			if err := finishAll(t, dir); err == nil {
				t.Fatalf("the barrier went past the unknown copy %s", report.ID)
			}
			stop := stopOn(t, dir, report)
			d := newDecision(t, dir, journal, epoch, report, c.Decided)
			d.Parent = parent
			d.Disposes = []Disposal{{Key: stop.key, Occurrence: stop.id}}
			writeDecision(t, dir, d)
			if c.Decided != ChoiceNotArrived {
				break
			}
			resent, err := d.resendJournal()
			if err != nil {
				t.Fatal(err)
			}
			journal, report, parent = ResendJournal(d.ID), resent.Reports[0], d.ID
		}
		if last := chain[len(chain)-1]; last.Decided == ChoiceNotArrived {
			// The copy the last decision sent is published as any report is.
			copies = append(copies, report)
			chain = append(chain, decisionmodel.Copy{Landed: true})
		}
		if err := finishAll(t, dir); err != nil {
			t.Fatalf("the decided chain stays stopped: %v", err)
		}
		want := decisionmodel.Expect(chain)
		letters := lettersOf(t, dir, "web")
		landings := 0
		for _, copied := range copies {
			if letters[copied.ID] > 1 {
				t.Fatalf("copy %s is in web %d times", copied.ID, letters[copied.ID])
			}
			landings += letters[copied.ID]
		}
		if len(copies) != want.Copies || landings != want.Landings {
			t.Fatalf("%d copies, %d landed; the model says %+v", len(copies), landings, want)
		}
		if left := stopsOnRecord(t, dir, "api"); len(left) != 0 {
			t.Fatalf("occurrences left open: %+v", left)
		}
	})
}
