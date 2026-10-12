package mail

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/registry/registrytest"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/proc"
)

// publishMain registers the room's main as the test's parent process, a live
// run that is neither the recipient's nor this process's.
func publishMain(t *testing.T, dir, namespace string) registry.Session {
	t.Helper()
	pid := os.Getppid()
	start, err := proc.StartTime(pid)
	if err != nil {
		t.Skipf("the parent's start cannot be read: %v", err)
	}
	lead := registry.Session{
		Name: "lead", Role: role.Main.ID, ServicePID: pid, ServiceStart: start, Boot: registrytest.Boot(t),
		PIDNamespace: namespace, CWD: dir, StartedAt: time.Now(),
	}
	if err := registry.Publish(dir, lead); err != nil {
		t.Fatal(err)
	}
	return lead
}

// The room's live main is told by a note frozen in the decision, whoever
// decided; the recipient is told unless its own run decided.
func TestDecideFreezesANoteToTheRoomsMain(t *testing.T) {
	for who, recipientNotes := range map[string]int{"by the person": 1, "by the recipient": 0} {
		t.Run(who, func(t *testing.T) {
			lab := newUnknownLab(t)
			lead := publishMain(t, lab.dir, "")
			by := DecidedBy{Kind: ByPerson}
			if who == "by the recipient" {
				by = lab.web()
			}
			answer, err := lab.decide(t, ChoiceArrived, by)
			if err != nil {
				t.Fatal(err)
			}
			told := map[string]DecisionNote{}
			for _, note := range answer.Decision.Notes {
				told[note.Told] = note
			}
			main, ok := told[ToldMain]
			if !ok || main.Note.To != lead.Name || main.Note.ToEpoch != lead.Epoch() || len(told) != 1+recipientNotes {
				t.Fatalf("the notes are %+v", answer.Decision.Notes)
			}
			if lettersOf(t, lab.dir, lead.Name)[main.Note.ID] != 1 {
				t.Fatal("main was not told")
			}
		})
	}
}

// A main whose run cannot be judged may be owed a note, so nothing is
// decided until it can.
func TestDecideRefusesWhileTheRoomsMainCannotBeJudged(t *testing.T) {
	lab := newUnknownLab(t)
	publishMain(t, lab.dir, "pid:[1]")
	before := wholeTree(t, lab.dir)
	if _, err := lab.decide(t, ChoiceArrived, DecidedBy{Kind: ByPerson}); outcomeOf(err) != DecisionUnreadable {
		t.Fatalf("an unjudgeable main answered %v", err)
	}
	if after := wholeTree(t, lab.dir); !sameTree(before, after) {
		t.Fatalf("a refused decision wrote: %v", treeDiff(before, after))
	}
}

// A report a session sent to itself is decided under its one mailbox lock,
// taken once.
func TestDecideOnAReportToItsOwnMailbox(t *testing.T) {
	dir := stateDir(t)
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	web := registry.Session{Name: "web", ServicePID: os.Getpid(), ServiceStart: start, Boot: registrytest.Boot(t), CWD: dir, StartedAt: time.Now()}
	if err := registry.Publish(dir, web); err != nil {
		t.Fatal(err)
	}
	report := Message{ID: NewID(), From: "web", FromEpoch: web.Epoch(), To: "web", ToEpoch: web.Epoch(), Kind: Note, Text: "to myself", CreatedAt: time.Now()}
	if err := WriteJournal(dir, "web", "end", TurnJournal{Epoch: web.Epoch(), Reports: []Message{report}}); err != nil {
		t.Fatal(err)
	}
	// A report to its own mailbox stops the barrier on the mailbox's own
	// mark, a record no operation names; the occurrence about the report is
	// written as a pass that read it as the report's would write it.
	mark := damageMark(t, dir, report)
	if _, err := live(dir).recordOccurrence("web", stopCause{Kind: causeUnreadable, Paths: []string{relative(dir, mark)}, Op: "end/" + report.ID, Cause: "the mark does not read"}); err != nil {
		t.Fatal(err)
	}
	subject, err := FindSubject(dir, "web", report.ID)
	if err != nil {
		t.Fatal(err)
	}
	by := DecidedBy{Kind: ByRecipient, Name: "web", Run: web.Epoch(), Verified: true}
	answer, err := Decide(context.Background(), dir, DecideRequest{Sender: "web", Report: report.ID, Choice: ChoiceNotArrived, By: by, Expect: subject})
	if err != nil || len(answer.Decision.Notes) != 0 || !live(dir).durable(answer.Path, answer.Decision) {
		t.Fatalf("decided as %+v, %v", answer, err)
	}
}
