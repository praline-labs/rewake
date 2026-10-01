package inbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/registry/registrytest"
	"github.com/praline-labs/rewake/internal/state"
)

// What the journal cannot recognize is unknown, never permission, and what it
// finds done is recorded before it goes on (docs/mail-bridge-cli.md, rule 8).

// A report found in the recipient's mailbox without its mark — written by an
// earlier build, which kept none — is marked published before the journal
// goes on: once the recipient has read it and the sweep has removed it, the
// mark is the only proof left.
func TestAReportFoundWithoutItsMarkIsMarkedPublished(t *testing.T) {
	dir, _, report := journalLab(t)
	if err := PutOnce(dir, report); err != nil {
		t.Fatal(err)
	}
	journals := JournalPath(dir, "api")
	if err := os.Chmod(journals, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(journals, 0o700) })
	if finishAll(t, dir) == nil {
		t.Fatal("the entry was recorded")
	}
	if err := os.Chmod(journals, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := archive(dir, "web", report.ID); err != nil {
		t.Fatal(err)
	}
	done := filepath.Join(state.DonePath(dir, "web"), report.ID+".json")
	old := time.Now().Add(-keepFinished - time.Hour)
	if err := os.Chtimes(done, old, old); err != nil {
		t.Fatal(err)
	}
	(&Server{Dir: dir, Name: "web", Epoch: report.ToEpoch}).sweepFinished()
	if _, err := os.Stat(done); !os.IsNotExist(err) {
		t.Fatalf("the sweep kept the report: %v", err)
	}
	if err := finishAll(t, dir); err != nil {
		t.Fatal(err)
	}
	if waiting, err := list(dir, "web"); err != nil || len(waiting) != 0 {
		t.Fatalf("republished %d (%v)", len(waiting), err)
	}
}

// A mark that says neither intent nor published stops whatever would rest on
// it: the journal's publication, a heads-up's, the sweep's removal of the
// letter, and the answer to what a run was sent.
func TestAnUnknownPublicationMarkIsNeverPermission(t *testing.T) {
	unknown := func(t *testing.T, dir string, report Message) string {
		t.Helper()
		path, _ := oncePath(dir, report.To, report.ToEpoch, report.ID)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("damaged"), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	t.Run("journal", func(t *testing.T) {
		dir, _, report := journalLab(t)
		unknown(t, dir, report)
		if err := finishAll(t, dir); err == nil {
			t.Fatal("the journal went on")
		}
		if waiting, err := list(dir, "web"); err != nil || len(waiting) != 0 {
			t.Fatalf("published %d (%v)", len(waiting), err)
		}
	})
	t.Run("heads-up", func(t *testing.T) {
		dir, _, report := journalLab(t)
		path := unknown(t, dir, report)
		if wrote, err := PublishOnce(context.Background(), dir, report, nil); err == nil || wrote {
			t.Fatalf("published: %v %v", wrote, err)
		}
		if mark, _ := os.ReadFile(path); string(mark) != "damaged" {
			t.Fatalf("the mark was written over: %q", mark)
		}
	})
	t.Run("sweep", func(t *testing.T) {
		dir, _, report := journalLab(t)
		unknown(t, dir, report)
		if err := PutOnce(dir, report); err != nil {
			t.Fatal(err)
		}
		if err := archive(dir, "web", report.ID); err != nil {
			t.Fatal(err)
		}
		done := filepath.Join(state.DonePath(dir, "web"), report.ID+".json")
		old := time.Now().Add(-keepFinished - time.Hour)
		if err := os.Chtimes(done, old, old); err != nil {
			t.Fatal(err)
		}
		(&Server{Dir: dir, Name: "web", Epoch: report.ToEpoch}).sweepFinished()
		if _, err := os.Stat(done); err != nil {
			t.Fatalf("the sweep removed the letter: %v", err)
		}
	})
	t.Run("publication", func(t *testing.T) {
		dir, _, report := journalLab(t)
		unknown(t, dir, report)
		if _, err := PublicationOf(context.Background(), dir, report.To, report.ToEpoch, report.ID); err == nil {
			t.Fatal("the mark was read as an answer")
		}
	})
}

// holdAPI registers api as the run epoch names, so the interim step can tell
// the run holding the name from an ended one.
func holdAPI(t *testing.T, dir string) string {
	t.Helper()
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	api := registry.Session{Name: "api", ServicePID: os.Getpid(), ServiceStart: start, Boot: registrytest.Boot(t), CWD: dir, StartedAt: time.Now()}
	if err := registry.Publish(dir, api); err != nil {
		t.Fatal(err)
	}
	return api.Epoch()
}

// noteInterim records an interim end of run epoch that ended at 1.
func noteInterim(t *testing.T, dir, epoch, text string) {
	t.Helper()
	if err := recordInterim(dir, "api", epoch, "noted", 1, &text, false); err != nil {
		t.Fatal(err)
	}
}

// An interim record that cannot be read stops the journal that would write
// it, whichever run's: it may be the live run's last word
// (docs/turn-end-recovery.md#the-interim-record).
func TestAnUnreadableInterimStopsTheJournal(t *testing.T) {
	for _, settles := range []bool{false, true} {
		for _, own := range []bool{false, true} {
			t.Run(map[bool]string{false: "interim", true: "settles"}[settles]+map[bool]string{false: ",ended", true: ",own"}[own], func(t *testing.T) {
				dir := stateDir(t)
				live := holdAPI(t, dir)
				noteInterim(t, dir, live, "the running work")
				path := interimPath(dir, "api")
				if err := os.Chmod(path, 0); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
				line := "the journal's work"
				journal := TurnJournal{Epoch: "ended-run", Op: "end", Ended: 5, Settles: settles}
				if own {
					journal.Epoch = live
				}
				if !settles {
					journal.Interim = &line
				}
				if err := WriteJournal(dir, "api", "end", journal); err != nil {
					t.Fatal(err)
				}
				err := finishAll(t, dir)
				_ = os.Chmod(path, 0o600)
				text, noted, readErr := LastInterim(dir, "api", live)
				if readErr != nil {
					t.Fatal(readErr)
				}
				if err == nil || !noted || text != "the running work" {
					t.Fatalf("finish %v; interim %q %v", err, text, noted)
				}
				if err := finishAll(t, dir); err != nil {
					t.Fatalf("the journal did not complete once the record was readable: %v", err)
				}
			})
		}
	}
}

// An interim end of the run holding the name is noted over the record an
// ended run left, which nobody will be asked about again.
func TestAnInterimEndReplacesAnEndedRunsRecord(t *testing.T) {
	dir := stateDir(t)
	live := holdAPI(t, dir)
	noteInterim(t, dir, "ended-run", "the ended run's work")
	line := "the running work"
	if err := WriteJournal(dir, "api", "end", TurnJournal{Epoch: live, Op: "end", Ended: 5, Interim: &line}); err != nil {
		t.Fatal(err)
	}
	if err := finishAll(t, dir); err != nil {
		t.Fatal(err)
	}
	if text, noted, err := LastInterim(dir, "api", live); err != nil || !noted || text != line {
		t.Fatalf("interim %q %v (%v)", text, noted, err)
	}
}
