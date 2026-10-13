package mail

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/registry"
)

// What the views read of a decision: before its record is durable nothing;
// durable with its tail not run, the wait still owed; after the tail, closed.
// A resend is named by the decision that sent it.
func TestDecisionMarksWhatTheViewsShow(t *testing.T) {
	for _, choice := range []string{ChoiceArrived, ChoiceNotArrived} {
		t.Run(choice, func(t *testing.T) {
			dir, epoch, report := journalLab(t)
			damageMark(t, dir, report)
			if err := finishAll(t, dir); err == nil {
				t.Fatal("the barrier went past an unknown mark")
			}
			stop := stopOn(t, dir, report)
			d := newDecision(t, dir, "end", epoch, report, choice)
			d.Disposes = []Disposal{{Key: stop.key, Occurrence: stop.id}}
			path := writeDecision(t, dir, d)
			if mark, ok := DecisionOn(dir, "api", "question", "web", report.ToEpoch); ok {
				t.Fatalf("a decision not yet durable marks the wait: %+v", mark)
			}
			if err := live(dir).makeDurable("api", path, d); err != nil {
				t.Fatal(err)
			}
			mark, ok := DecisionOn(dir, "api", "question", "web", report.ToEpoch)
			if !ok || mark.Closed || mark.Decision != d.ID || mark.Report != report.ID || mark.Choice != choice {
				t.Fatalf("decided, tail not run: %+v %v", mark, ok)
			}
			if _, ok := DecisionOn(dir, "api", "question", "web", "another-run"); ok {
				t.Fatal("a decision marks the wait of another run")
			}
			if err := finishAll(t, dir); err != nil {
				t.Fatal(err)
			}
			if mark, ok = DecisionOn(dir, "api", "question", "web", report.ToEpoch); !ok || !mark.Closed {
				t.Fatalf("after the tail: %+v %v", mark, ok)
			}
			resent, found := ResentBy(dir, "api", d.Resent())
			if (choice == ChoiceNotArrived) != found || found && resent.Decision != d.ID {
				t.Fatalf("resent %q by %+v, found %v", d.Resent(), resent, found)
			}
			if _, found := ResentBy(dir, "api", report.ID); found {
				t.Fatal("the decided report reads as a resend")
			}
		})
	}
}

// Main's note of a stop names who decides it and the two lines that do; a
// stop no decision settles says so.
func TestTheStopNoteSaysWhoDecides(t *testing.T) {
	t.Run("a subject", func(t *testing.T) {
		dir, _, report := journalLab(t)
		web, err := registry.LookupReadOnly(dir, "web")
		if err != nil {
			t.Fatal(err)
		}
		lead := web
		lead.Name, lead.Role = "lead", "main"
		if err := registry.Publish(dir, lead); err != nil {
			t.Fatal(err)
		}
		damageMark(t, dir, report)
		if err := finishAll(t, dir); err == nil {
			t.Fatal("the barrier went past an unknown mark")
		}
		note := noteTo(t, dir, "lead", stopOn(t, dir, report).id)
		for _, want := range []string{
			"web (run " + web.Epoch() + ") decides whether report " + report.ID + " reached it",
			"rewake decide api " + report.ID + " --arrived", "rewake decide api " + report.ID + " --not-arrived",
		} {
			if !strings.Contains(note, want) {
				t.Errorf("the note lacks %q:\n%s", want, note)
			}
		}
	})
	t.Run("no subject", func(t *testing.T) {
		lab := newTwoSessionLab(t)
		writeRaw(t, interimPath(lab.dir, "api"), "{")
		if MailboxStopped(lab.dir, "api") == nil {
			t.Fatal("an unreadable interim record left the mailbox open")
		}
		note := noteTo(t, lab.dir, "lead", stopsOnRecord(t, lab.dir, "api")[0].id)
		if !strings.Contains(note, "No decision settles it") || strings.Contains(note, "rewake decide") {
			t.Fatalf("the note:\n%s", note)
		}
	})
}

func noteTo(t *testing.T, dir, name, id string) string {
	t.Helper()
	letters, err := List(dir, name)
	if err != nil {
		t.Fatal(err)
	}
	for _, letter := range letters {
		if letter.ID == id {
			return letter.Text
		}
	}
	t.Fatalf("no note %s to %s", id, name)
	return ""
}
