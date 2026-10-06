package inbox

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// A gate whose plan found an unknown and whose occurrence of that stop could
// not be recorded still answers the stop, with the cause and the failed
// write: every later gate plans again and finds the cause while it is there,
// and records it then. Removing the cause afterwards is no evidence: the
// mailbox stays stopped. Valid bytes closed to reading and opened again are,
// and the mailbox opens.
func TestAGateWhoseStopCannotBeWrittenStillStops(t *testing.T) {
	for _, op := range []string{"make stops", "make key", "publish"} {
		for _, bytes := range []string{"invalid, removed", "valid, closed and reopened"} {
			t.Run(op+"/"+bytes, func(t *testing.T) {
				lab := newTwoSessionLab(t)
				interim := interimPath(lab.dir, "api")
				valid := bytes != "invalid, removed"
				if valid {
					raw, err := json.Marshal(interimRecord{Epoch: lab.run, Op: "earlier", Text: "later"})
					if err != nil {
						t.Fatal(err)
					}
					writeRaw(t, interim, string(raw))
					closeToReading(t, interim)
				} else {
					writeRaw(t, interim, "{")
				}
				rel, err := filepath.Rel(lab.dir, interim)
				if err != nil {
					t.Fatal(err)
				}
				key := stopCause{Kind: causeUnreadable, Paths: []string{rel}}.key()
				at := map[string]seamWrite{
					"make stops": {op: "make", rel: filepath.Join("inbox", "api", stopsDir), n: 1},
					"make key":   {op: "make", rel: filepath.Join("inbox", "api", stopsDir, key), n: 1},
					"publish":    {op: "publish", rel: filepath.Join("inbox", "api", stopsDir, key, "<occurrence>"), n: 1},
				}[op]
				probe := newProbe(lab.dir, osAccess{}, nil)
				probe.broken = &at
				testAccess.Store(lab.dir, passAccess{live: probe, plan: osAccess{}})
				t.Cleanup(func() { testAccess.Delete(lab.dir) })
				err = MailboxStopped(lab.dir, "api")
				if !probe.broke || !errors.Is(err, syscall.EIO) || !strings.Contains(err.Error(), interim) || !strings.Contains(err.Error(), "could not record the stop") {
					t.Fatalf("with its record broken at %s the gate answers %v (broken: %v)", op, err, probe.broke)
				}
				testAccess.Delete(lab.dir)
				if len(stopsOnRecord(t, lab.dir, "api")) != 0 {
					t.Fatal("an occurrence is on record past its broken write")
				}
				if err := MailboxStopped(lab.dir, "api"); err == nil {
					t.Fatal("a later gate went past the cause its plan still finds")
				}
				if len(stopsOnRecord(t, lab.dir, "api")) != 1 {
					t.Fatal("the later gate did not record the occurrence")
				}
				if valid {
					readable(t, interim)
					if err := MailboxStopped(lab.dir, "api"); err != nil {
						t.Fatalf("with its record readable again the mailbox stays stopped: %v", err)
					}
					return
				}
				if err := os.Remove(interim); err != nil {
					t.Fatal(err)
				}
				if err := MailboxStopped(lab.dir, "api"); !removedWhileStopped(err, interim) {
					t.Fatalf("with its cause removed the gate answers %v", err)
				}
			})
		}
	}
}

// No stop record is ever removed: an occurrence closes by a resolution of its
// own. One whose write fails reaches the caller, by the gate or the barrier,
// of a reading stop or an effect's, and the occurrence stays open; the next
// barrier writes it, and the mailbox opens. An effect's occurrence is
// resolved only after the effects ran through, so a failed resolution lets
// past no effect left to make.
func TestAFailedResolutionReachesTheCaller(t *testing.T) {
	for _, caller := range []string{"gate", "barrier, a reading stop", "barrier, an effect's stop"} {
		t.Run(caller, func(t *testing.T) {
			lab := newTwoSessionLab(t)
			version := "v1"
			raw, err := json.Marshal(keptRecord{Epoch: lab.run, Text: "held", Version: version})
			if err != nil {
				t.Fatal(err)
			}
			kept := keptPath(lab.dir, "api")
			writeRaw(t, kept, string(raw))
			rel, err := filepath.Rel(lab.dir, kept)
			if err != nil {
				t.Fatal(err)
			}
			cause := stopCause{Kind: causeUnreadable, Paths: []string{rel}, Cause: "an earlier cause"}
			if caller == "barrier, an effect's stop" {
				if err := WriteJournal(lab.dir, "api", "end", TurnJournal{Epoch: lab.run, Op: "end", Kept: &version}); err != nil {
					t.Fatal(err)
				}
				cause.Kind, cause.Op = causeEffect, "end"
			}
			stop, err := live(lab.dir).recordOccurrence("api", cause)
			if err != nil {
				t.Fatal(err)
			}
			probe := newProbe(lab.dir, osAccess{}, nil)
			probe.broken = &seamWrite{op: "publish", rel: filepath.Join("inbox", "api", stopsDir, stop.key, "<occurrence>"+resolvedSuffix), n: 1}
			testAccess.Store(lab.dir, passAccess{live: probe, plan: osAccess{}})
			t.Cleanup(func() { testAccess.Delete(lab.dir) })
			if caller == "gate" {
				err = MailboxStopped(lab.dir, "api")
			} else {
				err = lab.reconcile(t)
			}
			if !probe.broke || !errors.Is(err, syscall.EIO) {
				t.Fatalf("a failed resolution answers %v (broken: %v)", err, probe.broke)
			}
			testAccess.Delete(lab.dir)
			if open := stopsOnRecord(t, lab.dir, "api"); len(open) != 1 || open[0].id != stop.id {
				t.Fatalf("after a failed resolution the occurrences open are %+v", open)
			}
			if err := lab.reconcile(t); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(stop.path(lab.dir, "api") + resolvedSuffix); err != nil {
				t.Fatalf("the next barrier did not write the resolution: %v", err)
			}
			if err := MailboxStopped(lab.dir, "api"); err != nil {
				t.Fatal(err)
			}
		})
	}
}
