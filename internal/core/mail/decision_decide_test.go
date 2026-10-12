package mail

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// unknownLab is journalLab's report stopped on: its mark damaged, the barrier
// stopped on it, an occurrence about it open. The report goes to web, whose
// run is this very process, so a verified decision is web's to make here.
type unknownLab struct {
	dir, epoch string
	report     Message
	mark       string
	stop       openStop
}

func newUnknownLab(t *testing.T) unknownLab {
	t.Helper()
	dir, epoch, report := journalLab(t)
	mark := damageMark(t, dir, report)
	if err := finishAll(t, dir); err == nil {
		t.Fatal("the barrier went past an unknown mark")
	}
	return unknownLab{dir: dir, epoch: epoch, report: report, mark: mark, stop: stopOn(t, dir, report)}
}

// web is the recipient run deciding, verified.
func (l unknownLab) web() DecidedBy {
	return DecidedBy{Kind: ByRecipient, Name: "web", Run: l.report.ToEpoch, Verified: true}
}

func (l unknownLab) ask(t *testing.T, choice string, by DecidedBy) DecideRequest {
	t.Helper()
	subject, err := FindSubject(l.dir, "api", l.report.ID)
	if err != nil {
		t.Fatalf("the subject is not found: %v", err)
	}
	return DecideRequest{Sender: "api", Report: l.report.ID, Choice: choice, By: by, Expect: subject}
}

func (l unknownLab) decide(t *testing.T, choice string, by DecidedBy) (DecideAnswer, error) {
	t.Helper()
	return Decide(context.Background(), l.dir, l.ask(t, choice, by))
}

func outcomeOf(err error) DecisionOutcome {
	var refusal *DecisionError
	if errors.As(err, &refusal) {
		return refusal.Outcome
	}
	return ""
}

// The transaction writes one decision on the unknown report and finishes
// what it fixed: the record durable, the exact occurrence disposed, the
// journal completed without the report, the resend once for "did not
// arrive", and the notes owed to every run but the one that decided,
// verified. Its observations are the reads it made: the journal, the
// occurrence, and the mark that does not read.
func TestDecideRecordsOneDecisionAndFinishesIt(t *testing.T) {
	people := map[string]func(unknownLab) DecidedBy{
		"by the person":    func(unknownLab) DecidedBy { return DecidedBy{Kind: ByPerson} },
		"by the recipient": unknownLab.web,
	}
	for _, choice := range []string{ChoiceArrived, ChoiceNotArrived} {
		for who, by := range people {
			t.Run(choice+" "+who, func(t *testing.T) {
				lab := newUnknownLab(t)
				answer, err := lab.decide(t, choice, by(lab))
				if err != nil || answer.Before || answer.Barrier != nil {
					t.Fatalf("decided as %+v, %v", answer, err)
				}
				d := answer.Decision
				if answer.Path != DecisionPath(lab.dir, "api", lab.report.ID) || !live(lab.dir).durable(answer.Path, d) {
					t.Fatalf("the decision is at %s, durable %v", answer.Path, live(lab.dir).durable(answer.Path, d))
				}
				if len(d.Disposes) != 1 || d.Disposes[0] != (Disposal{Key: lab.stop.key, Occurrence: lab.stop.id}) {
					t.Fatalf("it disposes %+v, not the occurrence %s/%s", d.Disposes, lab.stop.key, lab.stop.id)
				}
				if left := stopsOnRecord(t, lab.dir, "api"); len(left) != 0 {
					t.Fatalf("occurrences left open: %+v", left)
				}
				if journal := readDone(t, lab.dir, "api", "end"); !journal.Done {
					t.Fatalf("the journal is %+v", journal)
				}
				letters := lettersOf(t, lab.dir, "web")
				if letters[lab.report.ID] != 0 {
					t.Fatal("the decided report was published")
				}
				wantNotes := map[string]int{"by the person": 1, "by the recipient": 0}[who]
				if len(d.Notes) != wantNotes || len(answer.Owed) != 0 {
					t.Fatalf("notes %+v, owed %v", d.Notes, answer.Owed)
				}
				for _, note := range d.Notes {
					if note.Told != ToldRecipient || letters[note.Note.ID] != 1 {
						t.Fatalf("the note %+v is in web %d times", note, letters[note.Note.ID])
					}
				}
				if choice == ChoiceNotArrived {
					resent, err := d.resendJournal()
					if err != nil || letters[resent.Reports[0].ID] != 1 {
						t.Fatalf("the resend is in web %d times: %v", letters[resent.Reports[0].ID], err)
					}
				}
				read := map[string]bool{}
				for _, o := range d.Observations {
					read[o.Op+" "+o.Path] = true
				}
				for _, want := range []string{
					"read " + relative(lab.dir, filepath.Join(JournalPath(lab.dir, "api"), "end")),
					"read " + relative(lab.dir, lab.stop.path(lab.dir, "api")),
					"read " + relative(lab.dir, lab.mark),
				} {
					if !read[want] {
						t.Fatalf("the observations miss %q: %v", want, read)
					}
				}
			})
		}
	}
}

// Asked again with the same words, the decision found is answered and its
// installation run; the other words are refused and replace nothing.
func TestDecideAgainKeepsTheFirstDecision(t *testing.T) {
	lab := newUnknownLab(t)
	first, err := lab.decide(t, ChoiceNotArrived, DecidedBy{Kind: ByPerson})
	if err != nil {
		t.Fatal(err)
	}
	before := wholeTree(t, lab.dir)
	again, err := lab.decide(t, ChoiceNotArrived, lab.web())
	if err != nil || !again.Before || again.Decision.ID != first.Decision.ID {
		t.Fatalf("the same words again answer %+v, %v", again, err)
	}
	if _, err := lab.decide(t, ChoiceArrived, DecidedBy{Kind: ByPerson}); outcomeOf(err) != DecisionOpposite {
		t.Fatalf("the other words answer %v", err)
	}
	if after := wholeTree(t, lab.dir); len(after) != len(before) || after[relative(lab.dir, first.Path)] != before[relative(lab.dir, first.Path)] {
		t.Fatal("asking again changed the decision or its effects")
	}
}

// Nothing is written when the decision is refused, whatever the reason.
func TestDecideRefusesWithNothingWritten(t *testing.T) {
	cases := map[string]struct {
		outcome DecisionOutcome
		ask     func(t *testing.T, lab unknownLab) DecideRequest
	}{
		"no open stop about the report": {DecisionNoSubject, func(t *testing.T, lab unknownLab) DecideRequest {
			req := lab.ask(t, ChoiceArrived, DecidedBy{Kind: ByPerson})
			req.Report = NewID()
			return req
		}},
		"a report id of no shape": {DecisionNoSubject, func(t *testing.T, lab unknownLab) DecideRequest {
			req := lab.ask(t, ChoiceArrived, DecidedBy{Kind: ByPerson})
			req.Report = "../end"
			return req
		}},
		"no choice": {DecisionNoSubject, func(t *testing.T, lab unknownLab) DecideRequest {
			return lab.ask(t, "maybe", DecidedBy{Kind: ByPerson})
		}},
		"another session as the recipient": {DecisionUnauthorized, func(t *testing.T, lab unknownLab) DecideRequest {
			by := lab.web()
			by.Name = "ops"
			return lab.ask(t, ChoiceArrived, by)
		}},
		"a verified run this process is not": {DecisionUnauthorized, func(t *testing.T, lab unknownLab) DecideRequest {
			by := lab.web()
			by.Run = "1.2"
			return lab.ask(t, ChoiceArrived, by)
		}},
		"the recipient's run asked of another process": {DecisionUnauthorized, func(t *testing.T, lab unknownLab) DecideRequest {
			// Right in every field, and asked of a process that is not the
			// run it names: only that run's wrapper writes as it.
			previous := selfRun
			selfRun = func() (string, error) { return "1.2", nil }
			t.Cleanup(func() { selfRun = previous })
			return lab.ask(t, ChoiceArrived, lab.web())
		}},
		"an unverified recipient": {DecisionUnauthorized, func(t *testing.T, lab unknownLab) DecideRequest {
			by := lab.web()
			by.Verified = false
			return lab.ask(t, ChoiceArrived, by)
		}},
		"a person naming a run": {DecisionUnauthorized, func(t *testing.T, lab unknownLab) DecideRequest {
			return lab.ask(t, ChoiceArrived, DecidedBy{Kind: ByPerson, Run: lab.report.ToEpoch})
		}},
		"a subject changed since it was found": {DecisionChanged, func(t *testing.T, lab unknownLab) DecideRequest {
			req := lab.ask(t, ChoiceArrived, DecidedBy{Kind: ByPerson})
			req.Expect.RecipientRun = "1.2"
			return req
		}},
		"published since the stop": {DecisionProven, func(t *testing.T, lab unknownLab) DecideRequest {
			writeRaw(t, lab.mark, oncePublished)
			return lab.ask(t, ChoiceArrived, DecidedBy{Kind: ByPerson})
		}},
		"the letter found since the stop": {DecisionProven, func(t *testing.T, lab unknownLab) DecideRequest {
			if err := live(lab.dir).put(lab.report); err != nil {
				t.Fatal(err)
			}
			return lab.ask(t, ChoiceNotArrived, DecidedBy{Kind: ByPerson})
		}},
		"not published, as proven since the stop": {DecisionProven, func(t *testing.T, lab unknownLab) DecideRequest {
			writeRaw(t, lab.mark, onceIntent)
			return lab.ask(t, ChoiceArrived, DecidedBy{Kind: ByPerson})
		}},
		"a decision there that does not read": {DecisionUnreadable, func(t *testing.T, lab unknownLab) DecideRequest {
			req := lab.ask(t, ChoiceArrived, DecidedBy{Kind: ByPerson})
			writeRaw(t, DecisionPath(lab.dir, "api", lab.report.ID), "{")
			return req
		}},
		"the stops cannot be listed": {DecisionUnreadable, func(t *testing.T, lab unknownLab) DecideRequest {
			req := lab.ask(t, ChoiceArrived, DecidedBy{Kind: ByPerson})
			stops := stopsPath(lab.dir, "api")
			if err := os.Chmod(stops, 0); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(stops, 0o700) })
			return req
		}},
		"the room's registry does not read": {DecisionUnreadable, func(t *testing.T, lab unknownLab) DecideRequest {
			req := lab.ask(t, ChoiceArrived, DecidedBy{Kind: ByPerson})
			writeRaw(t, filepath.Join(state.SessionsPath(lab.dir), "ops.json"), "{")
			return req
		}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if os.Geteuid() == 0 && name == "the stops cannot be listed" {
				t.Skip("root lists a directory of mode 0")
			}
			lab := newUnknownLab(t)
			req := c.ask(t, lab)
			before := wholeTree(t, lab.dir)
			answer, err := Decide(context.Background(), lab.dir, req)
			if outcomeOf(err) != c.outcome {
				t.Fatalf("answered %+v, %v; want %s", answer, err, c.outcome)
			}
			if after := wholeTree(t, lab.dir); !sameTree(before, after) {
				t.Fatalf("a refused decision wrote: %v", treeDiff(before, after))
			}
		})
	}
}

// A mark removed while stopped is no evidence that the report was not
// published: the decision is still the only way out.
func TestDecideTakesAMarkRemovedWhileStoppedForUnknown(t *testing.T) {
	lab := newUnknownLab(t)
	if err := os.Remove(lab.mark); err != nil {
		t.Fatal(err)
	}
	if _, err := lab.decide(t, ChoiceNotArrived, DecidedBy{Kind: ByPerson}); err != nil {
		t.Fatalf("a removed mark was taken for evidence: %v", err)
	}
}

// A sender's mailbox held past the wait answers busy and writes nothing.
func TestDecideOnABusyMailboxWritesNothing(t *testing.T) {
	lab := newUnknownLab(t)
	previous := senderLockWait
	senderLockWait = 50 * 1000 * 1000
	t.Cleanup(func() { senderLockWait = previous })
	req := lab.ask(t, ChoiceArrived, DecidedBy{Kind: ByPerson})
	held, release := make(chan struct{}), make(chan struct{})
	go func() {
		_ = state.WithMailboxLock(context.Background(), lab.dir, "api", func() error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	before := wholeTree(t, lab.dir)
	_, err := Decide(context.Background(), lab.dir, req)
	close(release)
	if outcomeOf(err) != DecisionBusy {
		t.Fatalf("a busy mailbox answered %v", err)
	}
	if after := wholeTree(t, lab.dir); !sameTree(before, after) {
		t.Fatalf("a busy mailbox was written: %v", treeDiff(before, after))
	}
}

// wholeTree is every file under dir, the stops among them, and every
// directory that cannot be listed as such: what a refused decision must leave
// as it was.
func wholeTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		if err != nil && entry != nil && entry.IsDir() {
			tree[rel] = "<unlistable>"
			return fs.SkipDir
		}
		if err != nil || rel == "." || filepath.Base(rel) == ".lock" {
			return err
		}
		if entry.IsDir() {
			tree[rel] = "<directory>"
			return nil
		}
		raw, err := os.ReadFile(path)
		tree[rel] = string(raw)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

func sameTree(a, b map[string]string) bool { return len(treeDiff(a, b)) == 0 }

func treeDiff(a, b map[string]string) []string {
	var diff []string
	for path, raw := range a {
		if other, ok := b[path]; !ok || other != raw {
			diff = append(diff, path)
		}
	}
	for path := range b {
		if _, ok := a[path]; !ok {
			diff = append(diff, path)
		}
	}
	return diff
}
