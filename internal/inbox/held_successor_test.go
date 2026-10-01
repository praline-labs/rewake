package inbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/registry/registrytest"
	"github.com/praline-labs/rewake/internal/state"
)

// A report of this build for a run of the earlier build is held, and the
// name's successor decides it (docs/protocol-cutover.md#what-this-build-refuses-or-holds).
// A sender's barrier runs between each two of the launch's steps 3, 4 and 5,
// and a crash at each of them: held while none is bound and while it starts,
// published once to it when it is ready, moot only once it is gone.

// holdReport leaves in web's journal a report for api's earlier run.
func (l conversionLab) holdReport(t *testing.T) Message {
	t.Helper()
	report := Message{ID: NewID(), From: "web", FromEpoch: l.web.Epoch(), To: "api", ToEpoch: earlierRun, Kind: Finished, Text: "HELD", CreatedAt: time.Now()}
	if err := WriteJournal(l.dir, "web", "end", TurnJournal{Epoch: l.web.Epoch(), Op: "end", Reports: []Message{report}}); err != nil {
		t.Fatal(err)
	}
	return report
}

func (l conversionLab) barrier(t *testing.T) {
	t.Helper()
	if err := withLock(l.dir, "web", func() error { return Reconcile(context.Background(), l.dir, "web") }); err != nil {
		t.Fatal(err)
	}
}

// bind is the launch's steps 2 and 3 for a run of api, of pid at start.
func (l conversionLab) bind(t *testing.T, pid int, start uint64) registry.Session {
	t.Helper()
	api := registry.Session{Name: "api", ServicePID: pid, ServiceStart: start, Boot: registrytest.Boot(t), PIDNamespace: proc.Namespace(), CWD: l.dir, StartedAt: time.Now()}
	if err := registry.WriteRunRecord(l.dir, registry.RunRecord{Name: "api", Boot: api.Boot, Epoch: api.Epoch(), Build: registry.BuildStamp, PIDNamespace: api.PIDNamespace}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.BindSuccessor(l.dir, "api", api.Epoch()); err != nil {
		t.Fatal(err)
	}
	return api
}

// taken lists api's letters that carry the held report.
func (l conversionLab) taken(t *testing.T, report Message) []Message {
	t.Helper()
	letters, err := list(l.dir, "api")
	if err != nil {
		t.Fatal(err)
	}
	var found []Message
	for _, letter := range letters {
		if letter.ID == report.ID {
			found = append(found, letter)
		}
	}
	return found
}

func withLock(dir, name string, fn func() error) error {
	return state.WithMailboxLock(context.Background(), dir, name, fn)
}

func TestAHeldReportFollowsTheSuccessor(t *testing.T) {
	lab := newConversionLab(t)
	report := lab.holdReport(t)
	lab.barrier(t)
	if len(lab.taken(t, report)) != 0 {
		t.Fatal("published with no successor bound")
	}
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	api := lab.bind(t, os.Getpid(), start)
	// Between steps 3 and 4: starting, so still held.
	lab.barrier(t)
	if len(lab.taken(t, report)) != 0 {
		t.Fatal("published to a successor still starting")
	}
	if err := registry.Publish(lab.dir, api); err != nil {
		t.Fatal(err)
	}
	// Between steps 4 and 5 the sender's barrier takes it; step 5 then adds
	// nothing.
	lab.barrier(t)
	TakeHeld(context.Background(), lab.dir, "api")
	taken := lab.taken(t, report)
	if len(taken) != 1 || taken[0].HeldFor != earlierRun || taken[0].ToEpoch != api.Epoch() {
		t.Fatalf("taken %+v", taken)
	}
	if lab.notes(t) != 0 {
		t.Fatal("main was told of a report that reached its successor")
	}
}

// Step 5 takes it itself when no sender's barrier ran since the successor
// became ready.
func TestTheLaunchTakesWhatIsHeldForItsName(t *testing.T) {
	lab := newConversionLab(t)
	report := lab.holdReport(t)
	lab.barrier(t)
	start, _ := proc.StartTime(os.Getpid())
	api := lab.bind(t, os.Getpid(), start)
	if err := registry.Publish(lab.dir, api); err != nil {
		t.Fatal(err)
	}
	TakeHeld(context.Background(), lab.dir, "api")
	if taken := lab.taken(t, report); len(taken) != 1 {
		t.Fatalf("step 5 took %d copies", len(taken))
	}
}

// A successor that crashed after binding is gone: the report is moot, the
// journal says so, main is told once, and nothing is published.
func TestAHeldReportIsMootOnlyOnceTheSuccessorIsGone(t *testing.T) {
	lab := newConversionLab(t)
	report := lab.holdReport(t)
	child := exec.Command("sleep", "60")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	start, err := proc.StartTime(child.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	lab.bind(t, child.Process.Pid, start)
	lab.barrier(t)
	if len(lab.taken(t, report)) != 0 || lab.notes(t) != 0 {
		t.Fatal("a living successor's report was decided")
	}
	_ = child.Process.Kill()
	_, _ = child.Process.Wait()
	for range 2 {
		lab.barrier(t)
	}
	if len(lab.taken(t, report)) != 0 || lab.notes(t) != 1 {
		t.Fatalf("taken %d, notes %d", len(lab.taken(t, report)), lab.notes(t))
	}
}

// chooseSuccessor records api's run as the recipient of the held report, as
// an attempt that died before publishing leaves it.
func (l conversionLab) chooseSuccessor(t *testing.T, report Message, api registry.Session) {
	t.Helper()
	path := filepath.Join(JournalPath(l.dir, "web"), "end")
	journal, err := readJournalFile(path)
	if err != nil {
		t.Fatal(err)
	}
	journal.Held = []string{report.ID}
	journal.Successors = map[string]string{report.ID: api.Epoch()}
	if err := writeJournalFile(path, journal); err != nil {
		t.Fatal(err)
	}
}

// A successor ready by its records whose session cannot take mail — its
// harness gone while its wrapper lingers — is no publication: the report
// stays held, and nothing is recorded as delivered. The name's lock is held
// through the barrier, as a launch or a cleanup holds it, so the dead record
// is not pruned before the look and the publication meets it.
func TestAHeldReportNobodyTookStaysHeld(t *testing.T) {
	lab := newConversionLab(t)
	report := lab.holdReport(t)
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	api := lab.bind(t, os.Getpid(), start)
	api.HarnessPID, api.HarnessStart = 4194003, 5
	if err := registry.Publish(lab.dir, api); err != nil {
		t.Fatal(err)
	}
	if err := state.WithNameLock(lab.dir, "api", func() error {
		for range 2 {
			lab.barrier(t)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	journal, err := readJournalFile(filepath.Join(JournalPath(lab.dir, "web"), "end"))
	if err != nil {
		t.Fatal(err)
	}
	if len(lab.taken(t, report)) != 0 || lab.notes(t) != 0 || !slices.Contains(journal.Held, report.ID) || slices.Contains(journal.Published, report.ID) {
		t.Fatalf("taken %d, notes %d, journal %+v", len(lab.taken(t, report)), lab.notes(t), journal)
	}
}

// A recorded successor whose state cannot be read now keeps the report held,
// naming nothing delivered: the recipient recorded is no proof it is ready.
func TestARecordedSuccessorOfUnknownStateKeepsTheReport(t *testing.T) {
	lab := newConversionLab(t)
	report := lab.holdReport(t)
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	api := lab.bind(t, os.Getpid(), start)
	if err := registry.Publish(lab.dir, api); err != nil {
		t.Fatal(err)
	}
	lab.chooseSuccessor(t, report, api)
	if err := os.WriteFile(filepath.Join(lab.dir, "runs", "api", api.Boot, api.Epoch()), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	lab.barrier(t)
	if len(lab.taken(t, report)) != 0 || lab.notes(t) != 0 {
		t.Fatalf("taken %d, notes %d", len(lab.taken(t, report)), lab.notes(t))
	}
}
