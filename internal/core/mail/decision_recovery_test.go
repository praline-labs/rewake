package mail

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// damageMark leaves report's publication mark saying neither intent nor
// published, so its publication is an unknown the barrier stops on.
func damageMark(t *testing.T, dir string, report Message) string {
	t.Helper()
	path, _ := oncePath(dir, report.To, report.ToEpoch, report.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	writeRaw(t, path, "damaged")
	return path
}

// stopOn is the occurrence open about report, failing the test when there is
// none.
func stopOn(t *testing.T, dir string, report Message) openStop {
	t.Helper()
	for _, stop := range stopsOnRecord(t, dir, report.From) {
		if strings.HasSuffix(stop.record.Op, "/"+report.ID) {
			return stop
		}
	}
	t.Fatalf("no occurrence open about %s: %+v", report.ID, stopsOnRecord(t, dir, report.From))
	return openStop{}
}

func readDone(t *testing.T, dir, name, id string) TurnJournal {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(JournalPath(dir, name), id+doneSuffix))
	if err != nil {
		t.Fatal(err)
	}
	var journal TurnJournal
	if err := json.Unmarshal(raw, &journal); err != nil {
		t.Fatal(err)
	}
	return journal
}

func lettersOf(t *testing.T, dir, name string) map[string]int {
	t.Helper()
	letters, err := List(dir, name)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]int{}
	for _, letter := range letters {
		ids[letter.ID]++
	}
	return ids
}

// A decision comes before every evidence of its report: the stop it disposes
// no longer holds the mailbox, the report is never published on it whether
// it landed or not, the tail runs and its closure is recorded, the note is
// told, and a resend goes out once in its own journal. Proof that arrives
// later changes neither the decision nor the journals it completed.
func TestDecisionPrecedesAllRecoveryEvidence(t *testing.T) {
	for _, choice := range []string{ChoiceArrived, ChoiceNotArrived} {
		for _, landed := range []bool{false, true} {
			name := choice
			if landed {
				name += ", landed"
			}
			t.Run(name, func(t *testing.T) {
				dir, epoch, report := journalLab(t)
				mark := damageMark(t, dir, report)
				if landed {
					if err := live(dir).put(report); err != nil {
						t.Fatal(err)
					}
				}
				if err := finishAll(t, dir); err == nil {
					t.Fatal("the barrier went past an unknown mark")
				}
				stop := stopOn(t, dir, report)
				d := newDecision(t, dir, "end", epoch, report, choice)
				d.Disposes = []Disposal{{Key: stop.key, Occurrence: stop.id}}
				writeDecision(t, dir, d)
				if err := finishAll(t, dir); err != nil {
					t.Fatalf("the decided mailbox stays stopped: %v", err)
				}
				letters := lettersOf(t, dir, "web")
				if want := map[bool]int{true: 1}[landed]; letters[report.ID] != want {
					t.Fatalf("the decided report is in web %d times, landed %v", letters[report.ID], landed)
				}
				if left := stopsOnRecord(t, dir, "api"); len(left) != 0 {
					t.Fatalf("occurrences left open: %+v", left)
				}
				// The done form keeps nothing of the steps; the closure says the
				// step was the decision's.
				if journal := readDone(t, dir, "api", "end"); !journal.Done {
					t.Fatalf("the journal completed as %+v", journal)
				}
				if waits, err := ReadWaiters(dir, "api", epoch); err != nil || len(waits) != 0 {
					t.Fatalf("the waits the tail clears stay: %v %v", waits, err)
				}
				closure := readClosureOf(t, dir, report.ID)
				if closure.Decision != d.ID || closure.Journal != "end" || len(closure.Waits) != 1 || closure.Waits[0].Name != "web" {
					t.Fatalf("the closure is %+v", closure)
				}
				if letters[d.Notes[0].Note.ID] != 1 || !fileExists(toldPath(dir, "api", report.ID, ToldRecipient)) {
					t.Fatalf("the note was told %d times, its record there: %v", letters[d.Notes[0].Note.ID], fileExists(toldPath(dir, "api", report.ID, ToldRecipient)))
				}
				if choice == ChoiceNotArrived {
					resent, _ := d.resendJournal()
					again := resent.Reports[0]
					summary := readDone(t, dir, "api", ResendJournal(d.ID)).Summary
					if letters[again.ID] != 1 || summary == nil || summary.Disposition != DispositionPublished || summary.Report != again.ID {
						t.Fatalf("the resend is in web %d times, summarized %+v", letters[again.ID], summary)
					}
				}

				// The proof arrives late: the mark now says published.
				writeRaw(t, mark, oncePublished)
				before := snapshot(t, dir)
				if err := finishAll(t, dir); err != nil {
					t.Fatal(err)
				}
				if after := snapshot(t, dir); !maps.Equal(before, after) {
					t.Fatal("a late proof changed what the decision completed")
				}
			})
		}
	}
}

func readClosureOf(t *testing.T, dir, report string) Closure {
	t.Helper()
	path := DecisionPath(dir, "api", report) + closedSuffix
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	closure, err := parseClosure(path, raw)
	if err != nil {
		t.Fatal(err)
	}
	return closure
}

// The installation waits for nothing the plan admits: with a journal beside
// it that does not read, before or after its own, the resend is installed
// and the note told while every journal stays where it was. A note that
// cannot be told stops nothing and is told by a later barrier, once.
func TestDecisionInstallationBeforeAdmission(t *testing.T) {
	for _, beside := range []string{"aaa", "zzz"} {
		t.Run(beside, func(t *testing.T) {
			lab := newDecisionLab(t)
			writeRaw(t, filepath.Join(JournalPath(lab.dir, "api"), beside), "{")
			d := lab.decision(t, ChoiceNotArrived)
			writeDecision(t, lab.dir, d)
			if err := finishAll(t, lab.dir); err == nil {
				t.Fatal("a journal that does not read was admitted")
			}
			if !live(lab.dir).resendNamed("api", d.ID) {
				t.Fatal("the resend waited for admission")
			}
			if !fileExists(toldPath(lab.dir, "api", lab.report.ID, ToldRecipient)) || lettersOf(t, lab.dir, "web")[d.Notes[0].Note.ID] != 1 {
				t.Fatal("the note waited for admission")
			}
			for _, id := range []string{"end", ResendJournal(d.ID)} {
				if fileExists(filepath.Join(JournalPath(lab.dir, "api"), id+doneSuffix)) {
					t.Fatalf("the journal %s ran in a stopped mailbox", id)
				}
			}
			if waits, err := ReadWaiters(lab.dir, "api", lab.epoch); err != nil || len(waits) != 1 {
				t.Fatalf("the waits moved in a stopped mailbox: %v %v", waits, err)
			}
		})
	}
	t.Run("a note that cannot be told", func(t *testing.T) {
		lab := newDecisionLab(t)
		d := lab.decision(t, ChoiceArrived)
		writeDecision(t, lab.dir, d)
		once := filepath.Join(state.InboxPath(lab.dir, "web"), "once")
		writeRaw(t, once, "not a directory")
		if err := finishAll(t, lab.dir); err != nil {
			t.Fatalf("a note owed stopped the mailbox: %v", err)
		}
		told := toldPath(lab.dir, "api", lab.report.ID, ToldRecipient)
		if fileExists(told) || !fileExists(filepath.Join(JournalPath(lab.dir, "api"), "end"+doneSuffix)) {
			t.Fatalf("told %v with the note not told, or the journal held by it", fileExists(told))
		}
		if err := os.Remove(once); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if err := finishAll(t, lab.dir); err != nil {
				t.Fatal(err)
			}
		}
		if !fileExists(told) || lettersOf(t, lab.dir, "web")[d.Notes[0].Note.ID] != 1 {
			t.Fatal("the note owed was not told once")
		}
	})
}

// What the installation and the tail stand on is read strictly: a told record
// that does not read, a decision that names another journal, bytes found
// under the resend's name and a summary that does not read each stop the
// mailbox as an unknown, publish nothing on it, and are never written over.
func TestDecisionRecoveryRecordsStopWhenUnknown(t *testing.T) {
	unknownStop := func(t *testing.T, lab decisionLab) {
		t.Helper()
		err := finishAll(t, lab.dir)
		var unknown *UnknownRecordError
		if !errors.As(err, &unknown) {
			t.Fatalf("the barrier answered %v, not an unknown", err)
		}
		if lettersOf(t, lab.dir, "web")[lab.report.ID] != 0 {
			t.Fatal("the report was published on an unknown")
		}
	}
	t.Run("told", func(t *testing.T) {
		lab := newDecisionLab(t)
		d := lab.decision(t, ChoiceArrived)
		writeDecision(t, lab.dir, d)
		told := toldPath(lab.dir, "api", lab.report.ID, ToldRecipient)
		writeRaw(t, told, "{")
		unknownStop(t, lab)
		if raw, _ := os.ReadFile(told); string(raw) != "{" || lettersOf(t, lab.dir, "web")[d.Notes[0].Note.ID] != 0 {
			t.Fatal("a told record that does not read was taken for owed")
		}
		if fileExists(filepath.Join(JournalPath(lab.dir, "api"), "end"+doneSuffix)) {
			t.Fatal("the journal ran past the installation's unknown")
		}
	})
	t.Run("another journal", func(t *testing.T) {
		lab := newDecisionLab(t)
		if err := WriteJournal(lab.dir, "api", "other", TurnJournal{Epoch: lab.epoch, Reports: []Message{lab.report}}); err != nil {
			t.Fatal(err)
		}
		d := newDecision(t, lab.dir, "other", lab.epoch, lab.report, ChoiceArrived)
		writeDecision(t, lab.dir, d)
		if err := os.Remove(filepath.Join(JournalPath(lab.dir, "api"), "other")); err != nil {
			t.Fatal(err)
		}
		unknownStop(t, lab)
	})
	t.Run("bytes under the resend's name", func(t *testing.T) {
		lab := newDecisionLab(t)
		d := lab.decision(t, ChoiceNotArrived)
		writeDecision(t, lab.dir, d)
		path := filepath.Join(JournalPath(lab.dir, "api"), ResendJournal(d.ID))
		writeRaw(t, path, "not the resend")
		unknownStop(t, lab)
		if raw, _ := os.ReadFile(path); string(raw) != "not the resend" {
			t.Fatal("the installation wrote over a name it found taken")
		}
	})
	t.Run("summary", func(t *testing.T) {
		lab := newDecisionLab(t)
		d := lab.decision(t, ChoiceNotArrived)
		writeDecision(t, lab.dir, d)
		writeResend(t, lab, d, &ResendSummary{Report: NewID(), Disposition: "lost", Decision: d.ID}, true)
		unknownStop(t, lab)
	})
}

// What a decision installed stays installed through retention: once the
// letters it put are read and swept, a barrier tells no note again, links no
// resend again and publishes no report again.
func TestDecisionEffectsAreNotRepeatedAfterRetention(t *testing.T) {
	lab := newDecisionLab(t)
	d := lab.decision(t, ChoiceNotArrived)
	writeDecision(t, lab.dir, d)
	if err := finishAll(t, lab.dir); err != nil {
		t.Fatal(err)
	}
	letters := lettersOf(t, lab.dir, "web")
	if len(letters) != 2 {
		t.Fatalf("web holds %v, not the note and the resend", letters)
	}
	for id := range letters {
		if err := archive(lab.dir, "web", id); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(state.DonePath(lab.dir, "web"), id+".json")); err != nil {
			t.Fatal(err)
		}
	}
	journals := func() []string {
		entries, err := os.ReadDir(JournalPath(lab.dir, "api"))
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		return names
	}
	before := journals()
	if err := finishAll(t, lab.dir); err != nil {
		t.Fatal(err)
	}
	if again := lettersOf(t, lab.dir, "web"); len(again) != 0 {
		t.Fatalf("a barrier after the sweep put %v again", again)
	}
	if after := journals(); !slices.Equal(before, after) {
		t.Fatalf("the journals went from %v to %v", before, after)
	}
}
