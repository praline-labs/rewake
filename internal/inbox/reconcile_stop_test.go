package inbox

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/state"
)

// The barrier reads every record before any effect: an unreadable journal
// beside one ready to publish stops both (8-stop). Removing the journal is
// no evidence of what it would have done, so the stop holds and says the
// journal was removed while stopped; the same journal, its valid bytes made
// unreadable and then readable again, lets both go once the plan reads it.
func TestAnUnreadableJournalStopsEveryEffect(t *testing.T) {
	for _, bytes := range []string{"invalid, removed", "valid, closed and reopened"} {
		t.Run(bytes, func(t *testing.T) {
			lab := newTwoSessionLab(t)
			report := lab.report(Finished, "task")
			if err := WriteJournal(lab.dir, "api", "a", TurnJournal{Epoch: lab.run, Reports: []Message{report}}); err != nil {
				t.Fatal(err)
			}
			broken := filepath.Join(JournalPath(lab.dir, "api"), "z")
			valid := bytes != "invalid, removed"
			if valid {
				if err := WriteJournal(lab.dir, "api", "z", TurnJournal{Epoch: lab.run, Op: "end"}); err != nil {
					t.Fatal(err)
				}
				closeToReading(t, broken)
			} else {
				writeRaw(t, broken, "{")
			}
			if lab.reconcile(t) == nil {
				t.Fatal("an unreadable journal went through")
			}
			if lab.copies(t, report.ID) != 0 {
				t.Fatal("a journal published beside an unreadable one")
			}
			if valid {
				readable(t, broken)
				if err := lab.reconcile(t); err != nil {
					t.Fatal(err)
				}
				if lab.copies(t, report.ID) != 1 || len(stopsOnRecord(t, lab.dir, "api")) != 0 {
					t.Fatalf("once readable: published %d times, %d occurrences open", lab.copies(t, report.ID), len(stopsOnRecord(t, lab.dir, "api")))
				}
				return
			}
			if err := os.Remove(broken); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				err := lab.reconcile(t)
				if !removedWhileStopped(err, broken) {
					t.Fatalf("with the journal removed the barrier answers %v", err)
				}
			}
			if lab.copies(t, report.ID) != 0 || len(stopsOnRecord(t, lab.dir, "api")) != 1 {
				t.Fatalf("the removal resolved the stop: published %d times", lab.copies(t, report.ID))
			}
		})
	}
}

// Every call that would change the mailbox asks the same question the barrier
// does: a journal or a wait record that cannot be read is a stop whichever call
// finds it, recorded as an occurrence. A record removed resolves nothing: the
// stop stands for every call, the barrier too, naming the removal. Valid bytes
// closed to reading and opened again are evidence, and the next look resolves
// the occurrence, naming what it read.
func TestAnUnreadableRecordStopsTheMailbox(t *testing.T) {
	for _, record := range []string{"journal", "wait"} {
		for _, bytes := range []string{"invalid, removed", "valid, closed and reopened"} {
			t.Run(record+"/"+bytes, func(t *testing.T) {
				lab := newTwoSessionLab(t)
				lab.owe(t, "task")
				path := filepath.Join(JournalPath(lab.dir, "api"), "broken")
				if record == "wait" {
					path = filepath.Join(state.AwaitingPath(lab.dir, "api"), lab.run, "web")
				}
				valid := bytes != "invalid, removed"
				switch {
				case valid && record == "journal":
					if err := WriteJournal(lab.dir, "api", "broken", TurnJournal{Epoch: lab.run, Op: "end"}); err != nil {
						t.Fatal(err)
					}
					closeToReading(t, path)
				case valid:
					closeToReading(t, path)
				default:
					// More fields than any wait record carries: it parses as none.
					writeRaw(t, path, "{ 1 2 3 4 5 6")
				}
				if MailboxStopped(lab.dir, "api") == nil {
					t.Fatalf("an unreadable %s did not stop the mailbox", record)
				}
				stops := stopsOnRecord(t, lab.dir, "api")
				if len(stops) != 1 {
					t.Fatalf("%d occurrences recorded", len(stops))
				}
				if valid {
					readable(t, path)
					if err := MailboxStopped(lab.dir, "api"); err != nil {
						t.Fatalf("a stop whose record reads again: %v", err)
					}
					raw, err := os.ReadFile(stops[0].path(lab.dir, "api") + resolvedSuffix)
					if err != nil || !strings.Contains(string(raw), "sha256") {
						t.Fatalf("the resolution names no evidence: %s %v", raw, err)
					}
					return
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := MailboxStopped(lab.dir, "api"); !removedWhileStopped(err, path) {
					t.Fatalf("with the %s removed the gate answers %v", record, err)
				}
				if err := lab.reconcile(t); !removedWhileStopped(err, path) {
					t.Fatalf("with the %s removed the barrier answers %v", record, err)
				}
				if len(stopsOnRecord(t, lab.dir, "api")) != 1 {
					t.Fatal("the removal resolved the occurrence")
				}
			})
		}
	}
}

// removedWhileStopped says err is the recorded stop, naming path as removed.
func removedWhileStopped(err error, path string) bool {
	var stopped *RecordedStopError
	return errors.As(err, &stopped) && strings.Contains(err.Error(), path+" was removed while stopped")
}
