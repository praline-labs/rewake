package mail

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// fakeProc describes the machine to the process reader for the rest of the
// test: this process's pid namespace, and pid's stat as stat says — or no
// process at all when stat is empty.
func fakeProc(t *testing.T, pid int, stat string) {
	t.Helper()
	original := proc.Default
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "self", "ns"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(original.Namespace(), filepath.Join(root, "self", "ns", "pid")); err != nil {
		t.Fatal(err)
	}
	if stat != "" {
		process := filepath.Join(root, strconv.Itoa(pid))
		if err := os.MkdirAll(process, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(process, "stat"), []byte(stat), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	proc.Default = proc.Reader{Root: root}
	t.Cleanup(func() { proc.Default = original })
}

// A note is told run-ended only on proof that its run ended (Q11). A run
// whose liveness cannot be told — its record there or gone, its process's
// stat unreadable — leaves the note owed, stops nothing and records nothing,
// so the barrier that can tell later tells it once. A process proven gone is
// told run-ended, and no letter is put.
func TestDecisionNoteToARunNotKnownToHaveEndedStaysOwed(t *testing.T) {
	for name, record := range map[string]bool{"its record there": true, "its record gone": false} {
		t.Run(name, func(t *testing.T) {
			lab := newDecisionLab(t)
			d := lab.decision(t, ChoiceArrived)
			writeDecision(t, lab.dir, d)
			peer, err := registry.Load(lab.dir, "web")
			if err != nil {
				t.Fatal(err)
			}
			if !record {
				if err := os.Remove(state.SessionPath(lab.dir, "web")); err != nil {
					t.Fatal(err)
				}
			}
			restore := proc.Default
			fakeProc(t, peer.ServicePID, "a stat that does not read")
			if got := registry.ObserveRun(peer.Epoch()); got != proc.IdentityUnknown {
				t.Fatalf("the run is %v, not unknown", got)
			}
			told := toldPath(lab.dir, "api", lab.report.ID, ToldRecipient)
			if err := finishAll(t, lab.dir); err != nil {
				t.Fatalf("a note owed stopped the barrier: %v", err)
			}
			if fileExists(told) || lettersOf(t, lab.dir, "web")[d.Notes[0].Note.ID] != 0 {
				t.Fatal("a note to a run not known to have ended was told")
			}
			proc.Default = restore
			if !record {
				if err := registry.Publish(lab.dir, peer); err != nil {
					t.Fatal(err)
				}
			}
			if err := finishAll(t, lab.dir); err != nil {
				t.Fatal(err)
			}
			if !fileExists(told) || lettersOf(t, lab.dir, "web")[d.Notes[0].Note.ID] != 1 {
				t.Fatal("the note owed was not told once its run could be told")
			}
		})
	}

	t.Run("its process gone", func(t *testing.T) {
		lab := newDecisionLab(t)
		d := lab.decision(t, ChoiceArrived)
		writeDecision(t, lab.dir, d)
		peer, err := registry.Load(lab.dir, "web")
		if err != nil {
			t.Fatal(err)
		}
		fakeProc(t, peer.ServicePID, "")
		if err := finishAll(t, lab.dir); err != nil {
			t.Fatal(err)
		}
		record, err := readTold(osAccess{}, toldPath(lab.dir, "api", lab.report.ID, ToldRecipient), d.Notes[0])
		if err != nil || record.Answer != ToldEnded || lettersOf(t, lab.dir, "web")[d.Notes[0].Note.ID] != 0 {
			t.Fatalf("a note to a run proven ended: %+v %v", record, err)
		}
	})
}
