package inbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/registry"
)

// validInterim leaves api a valid interim record closed to reading, and
// answers its path.
func validInterim(t *testing.T, lab twoSessionLab) string {
	t.Helper()
	interim := interimPath(lab.dir, "api")
	raw, err := json.Marshal(interimRecord{Epoch: lab.run, Op: "earlier", Text: "later"})
	if err != nil {
		t.Fatal(err)
	}
	writeRaw(t, interim, string(raw))
	closeToReading(t, interim)
	return interim
}

// notesTo counts the letters with id in name's mailbox.
func notesTo(t *testing.T, dir, name, id string) int {
	t.Helper()
	letters, err := list(dir, name)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, letter := range letters {
		if letter.ID == id {
			count++
		}
	}
	return count
}

// Two causes are two occurrences, and resolving one resolves nothing else:
// the stop holds while the other is open.
func TestTwoCausesOneResolvedTheStopHolds(t *testing.T) {
	lab := newTwoSessionLab(t)
	interim := validInterim(t, lab)
	if err := WriteJournal(lab.dir, "api", "z", TurnJournal{Epoch: lab.run, Op: "end"}); err != nil {
		t.Fatal(err)
	}
	journal := filepath.Join(JournalPath(lab.dir, "api"), "z")
	closeToReading(t, journal)
	if MailboxStopped(lab.dir, "api") == nil || len(stopsOnRecord(t, lab.dir, "api")) != 2 {
		t.Fatalf("two unreadable records: %d occurrences", len(stopsOnRecord(t, lab.dir, "api")))
	}
	readable(t, interim)
	err := MailboxStopped(lab.dir, "api")
	stops := stopsOnRecord(t, lab.dir, "api")
	if err == nil || len(stops) != 1 || !strings.Contains(stops[0].record.Cause, journal) {
		t.Fatalf("with one cause resolved the gate answers %v, %d occurrences open", err, len(stops))
	}
	readable(t, journal)
	if err := MailboxStopped(lab.dir, "api"); err != nil {
		t.Fatalf("with both causes resolved: %v", err)
	}
}

// Main is told of an occurrence once, through a letter under the
// occurrence's id: a key found again by gates and barriers while it is open
// is that occurrence, and tells nobody again.
func TestMainIsToldOnceAcrossBarriersAndCalls(t *testing.T) {
	lab := newTwoSessionLab(t)
	writeRaw(t, interimPath(lab.dir, "api"), "{")
	for range 2 {
		if MailboxStopped(lab.dir, "api") == nil || lab.reconcile(t) == nil {
			t.Fatal("an unreadable interim record left the mailbox open")
		}
	}
	stops := stopsOnRecord(t, lab.dir, "api")
	if len(stops) != 1 || !stops[0].told {
		t.Fatalf("occurrences open: %+v", stops)
	}
	if notes := notesTo(t, lab.dir, "lead", stops[0].id); notes != 1 {
		t.Fatalf("main was told %d times", notes)
	}
}

// A stopped mailbox changes nothing of its own, but letters from others
// still arrive in it.
func TestLettersArriveIntoAStoppedMailbox(t *testing.T) {
	lab := newTwoSessionLab(t)
	writeRaw(t, interimPath(lab.dir, "api"), "{")
	if MailboxStopped(lab.dir, "api") == nil {
		t.Fatal("an unreadable interim record left the mailbox open")
	}
	letter := Message{ID: NewID(), From: "web", FromEpoch: lab.web.Epoch(), To: "api", ToEpoch: lab.run, Kind: Task, Text: "arrives", CreatedAt: time.Now()}
	if err := Put(lab.dir, letter); err != nil {
		t.Fatal(err)
	}
	if notesTo(t, lab.dir, "api", letter.ID) != 1 {
		t.Fatal("a letter did not arrive into a stopped mailbox")
	}
}

// A record that names no operation and that its owner rewrites each turn:
// unreadable, rewritten valid and resolved, unreadable again — a new
// occurrence, which needs evidence of its own and tells main again — then
// removed, which resolves nothing.
func TestARecurringCauseOfARecordIsANewOccurrence(t *testing.T) {
	lab := newTwoSessionLab(t)
	interim := interimPath(lab.dir, "api")
	writeRaw(t, interim, "{")
	_ = MailboxStopped(lab.dir, "api")
	first := stopsOnRecord(t, lab.dir, "api")
	raw, err := json.Marshal(interimRecord{Epoch: lab.run, Op: "end", Text: "later"})
	if err != nil {
		t.Fatal(err)
	}
	writeRaw(t, interim, string(raw))
	if err := MailboxStopped(lab.dir, "api"); err != nil || len(first) != 1 {
		t.Fatalf("rewritten valid: %v, %d occurrences first", err, len(first))
	}
	writeRaw(t, interim, "{")
	_ = MailboxStopped(lab.dir, "api")
	second := stopsOnRecord(t, lab.dir, "api")
	if len(second) != 1 || second[0].key != first[0].key || second[0].id == first[0].id {
		t.Fatalf("unreadable again: %+v after %+v", second, first)
	}
	if notesTo(t, lab.dir, "lead", first[0].id) != 1 || notesTo(t, lab.dir, "lead", second[0].id) != 1 {
		t.Fatal("main was not told of each occurrence once")
	}
	if err := os.Remove(interim); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := lab.reconcile(t); !removedWhileStopped(err, interim) {
			t.Fatalf("with the record removed the barrier answers %v", err)
		}
	}
	if open := stopsOnRecord(t, lab.dir, "api"); len(open) != 1 || open[0].id != second[0].id {
		t.Fatalf("the second occurrence did not stay open: %+v", open)
	}
}

// A cause about an operation — the recipient's proof that api's report
// landed — unreadable, readable again and resolved, unreadable again: a new
// occurrence, about the same report; then removed, and the second occurrence
// stays open. The report is neither published again nor recorded moot.
func TestARecurringCauseAboutAnOperationIsANewOccurrence(t *testing.T) {
	lab := newTwoSessionLab(t)
	lab.owe(t, "t1")
	report := lab.report(Finished, "t1")
	if _, err := PublishOnce(t.Context(), lab.dir, report, nil); err != nil {
		t.Fatal(err)
	}
	lab.endTurn(t, report, "t1")
	mark, _ := oncePath(lab.dir, "web", lab.web.Epoch(), report.ID)
	if _, err := os.Stat(mark); err != nil {
		t.Fatalf("the proof of the landing: %v", err)
	}
	closeToReading(t, mark)
	_ = MailboxStopped(lab.dir, "api")
	first := stopsOnRecord(t, lab.dir, "api")
	if len(first) != 1 || first[0].reportOf() != report.ID {
		t.Fatalf("occurrences: %+v", first)
	}
	readable(t, mark)
	if err := MailboxStopped(lab.dir, "api"); err != nil {
		t.Fatalf("with the proof readable: %v", err)
	}
	closeToReading(t, mark)
	_ = MailboxStopped(lab.dir, "api")
	second := stopsOnRecord(t, lab.dir, "api")
	if len(second) != 1 || second[0].key != first[0].key || second[0].id == first[0].id {
		t.Fatalf("unreadable again: %+v after %+v", second, first)
	}
	readable(t, mark)
	if err := os.Remove(mark); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := lab.reconcile(t); !removedWhileStopped(err, mark) {
			t.Fatalf("with the proof removed the barrier answers %v", err)
		}
	}
	journal, err := live(lab.dir).readJournalFile(filepath.Join(JournalPath(lab.dir, "api"), "end"))
	if err != nil || len(journal.Published) > 0 || len(journal.Moot) > 0 || lab.copies(t, report.ID) != 1 {
		t.Fatalf("the report moved past an open occurrence: %+v %v, %d copies", journal, err, lab.copies(t, report.ID))
	}
}

// The recipient's run ends while an occurrence about its report is open:
// that the run ended says nothing of whether the report landed, so the report
// stays where it is — not recorded moot, the journal unfinished — and the
// stop holds across barriers, each call naming the report unsettled. The run
// ends before the barrier, which its plan sees, or between the plan and the
// pass after it, which only the pass sees.
func TestARecipientsEndLeavesAHeldReportUnsettled(t *testing.T) {
	for _, when := range []string{"before the barrier", "after its plan"} {
		t.Run(when, func(t *testing.T) {
			lab := newTwoSessionLab(t)
			lab.owe(t, "t1")
			report := lab.report(Finished, "t1")
			lab.endTurn(t, report, "t1")
			at, _ := oncePath(lab.dir, "web", lab.web.Epoch(), report.ID)
			mark, err := filepath.Rel(lab.dir, at)
			if err != nil {
				t.Fatal(err)
			}
			stop, err := live(lab.dir).recordOccurrence("api", stopCause{Kind: causeEffect, Paths: []string{mark}, Op: "end/" + report.ID, Cause: "the proof of the landing did not read"})
			if err != nil {
				t.Fatal(err)
			}
			end := func() {
				if _, err := registry.RemoveOwned(lab.dir, "web", lab.web.Epoch()); err != nil {
					t.Error(err)
				}
			}
			if when == "before the barrier" {
				end()
			} else {
				t.Cleanup(func() { afterReading = func() {} })
				afterReading = func() { afterReading = func() {}; end() }
			}
			for range 2 {
				if lab.reconcile(t) == nil {
					t.Fatal("a report held by an open occurrence let the barrier through")
				}
				if err := MailboxStopped(lab.dir, "api"); err == nil || !strings.Contains(err.Error(), report.ID+" stays unsettled") {
					t.Fatalf("the gate past a held report answers %v", err)
				}
			}
			journal, err := live(lab.dir).readJournalFile(filepath.Join(JournalPath(lab.dir, "api"), "end"))
			if err != nil || len(journal.Moot) > 0 || len(journal.Published) > 0 {
				t.Fatalf("the report was settled past an open occurrence: %+v %v", journal, err)
			}
			if open := stopsOnRecord(t, lab.dir, "api"); len(open) != 1 || open[0].id != stop.id {
				t.Fatalf("occurrences open: %+v", open)
			}
		})
	}
}
