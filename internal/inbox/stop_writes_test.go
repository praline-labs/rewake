package inbox

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// A gate whose plan found an unknown and whose record of that stop could not
// be made or written still answers the stop, with the cause and the failed
// write: the stop never depended on its record, since every later gate plans
// again and finds the cause while it is there. Once it is gone, the mailbox
// opens.
func TestAGateWhoseStopCannotBeWrittenStillStops(t *testing.T) {
	for _, op := range []string{"make", "write"} {
		t.Run(op, func(t *testing.T) {
			lab := newTwoSessionLab(t)
			interim := interimPath(lab.dir, "api")
			writeRaw(t, interim, "{")
			rel := filepath.Join("inbox", "api", stopFile)
			if op == "make" {
				rel = filepath.Dir(rel)
			}
			probe := newProbe(lab.dir, osAccess{}, nil)
			probe.broken = &seamWrite{op: op, rel: rel, n: 1}
			testAccess.Store(lab.dir, passAccess{live: probe, plan: osAccess{}})
			t.Cleanup(func() { testAccess.Delete(lab.dir) })
			err := MailboxStopped(lab.dir, "api")
			if !probe.broke || !errors.Is(err, syscall.EIO) || !strings.Contains(err.Error(), interim) {
				t.Fatalf("with its record broken by %s the gate answers %v (broken: %v)", op, err, probe.broke)
			}
			testAccess.Delete(lab.dir)
			if err := MailboxStopped(lab.dir, "api"); err == nil {
				t.Fatal("a later gate went past the cause its plan still finds")
			}
			if err := os.Remove(interim); err != nil {
				t.Fatal(err)
			}
			if err := MailboxStopped(lab.dir, "api"); err != nil {
				t.Fatalf("with its cause gone the mailbox stays stopped: %v", err)
			}
		})
	}
}

// A lift of a stop that fails reaches the caller, by the gate or the barrier,
// of a reading stop or an effect's: a remove that failed leaves the record in
// place, a sync that failed comes after the record is gone, and either way
// the next barrier and gate go through. An effect's stop is lifted only after
// the effects ran through, so a record gone before its sync failed lets past
// no effect left to make.
func TestALiftOfAStopThatFailsReachesTheCaller(t *testing.T) {
	for _, caller := range []string{"gate", "barrier, a reading stop", "barrier, an effect's stop"} {
		for _, op := range []string{"remove", "sync"} {
			t.Run(caller+"/"+op, func(t *testing.T) {
				lab := newTwoSessionLab(t)
				met := ""
				if caller == "barrier, an effect's stop" {
					met = "an earlier effect's cause"
				}
				if err := recordStop(lab.dir, "api", errors.New("an earlier cause"), met); err != nil {
					t.Fatal(err)
				}
				rel := filepath.Join("inbox", "api", stopFile)
				if op == "sync" {
					rel = filepath.Dir(rel)
				}
				probe := newProbe(lab.dir, osAccess{}, nil)
				probe.broken = &seamWrite{op: op, rel: rel, n: 1}
				testAccess.Store(lab.dir, passAccess{live: probe, plan: osAccess{}})
				t.Cleanup(func() { testAccess.Delete(lab.dir) })
				var err error
				if caller == "gate" {
					err = MailboxStopped(lab.dir, "api")
				} else {
					err = lab.reconcile(t)
				}
				if !probe.broke || !errors.Is(err, syscall.EIO) {
					t.Fatalf("a failed %s of the stop answers %v (broken: %v)", op, err, probe.broke)
				}
				testAccess.Delete(lab.dir)
				recorded, err := recordedStop(lab.dir, "api")
				if err != nil || (recorded != nil) != (op == "remove") {
					t.Fatalf("after a failed %s the stop on record is %+v %v", op, recorded, err)
				}
				if err := lab.reconcile(t); err != nil {
					t.Fatal(err)
				}
				if err := MailboxStopped(lab.dir, "api"); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
