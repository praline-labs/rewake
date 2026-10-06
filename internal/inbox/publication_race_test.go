package inbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/registry/registrytest"
	"github.com/praline-labs/rewake/internal/state"
)

// A once-publication reads its recipient's run, the evidence of an earlier
// landing and writes in one section of the recipient's lock, and the proof is
// retired only under that lock by the name's live run
// (docs/v2/stage3-publication.md). The race it closes: a retry admitted to a
// run that then ends, whose next run sweeps the old letter and marks before
// the retry reads them, wrote the report a second time.

// endableRun is a run of a name served by a process of its own, which end
// stops and reaps: from then on the run has ended.
type endableRun struct {
	session registry.Session
	child   *exec.Cmd
	once    sync.Once
}

func startRun(t *testing.T, dir, name string) *endableRun {
	t.Helper()
	child := exec.Command("sleep", "60")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	start, err := proc.StartTime(child.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	run := &endableRun{child: child, session: registry.Session{Name: name, ServicePID: child.Process.Pid, ServiceStart: start, Boot: registrytest.Boot(t), CWD: dir, StartedAt: time.Now()}}
	t.Cleanup(run.end)
	if err := registry.Publish(dir, run.session); err != nil {
		t.Fatal(err)
	}
	return run
}

func (r *endableRun) epoch() string { return r.session.Epoch() }

func (r *endableRun) end() {
	r.once.Do(func() {
		_ = r.child.Process.Kill()
		_ = r.child.Wait()
	})
}

// raceLab is the setup both schedules share: a report R of api's journal to
// lead's run r has landed, api died before saving Published, r has read R,
// and R's age has passed the sweep's cutoff.
type raceLab struct {
	twoSessionLab
	lead    *endableRun
	report  Message
	letters *letterWrites
}

func newRaceLab(t *testing.T) raceLab {
	t.Helper()
	lab := newBareRaceLab(t)
	lab.landRead(t, lab.report)
	return lab
}

// newBareRaceLab is the setup before the landing: api's journal of the end
// owes R to lead's live run r, and nothing has been published.
func newBareRaceLab(t *testing.T) raceLab {
	t.Helper()
	lab := raceLab{twoSessionLab: newTwoSessionLab(t)}
	// The lab's lead is served by this process, which never ends: r is a run
	// of its own here.
	if err := registry.Remove(lab.dir, "lead"); err != nil {
		t.Fatal(err)
	}
	lab.lead = startRun(t, lab.dir, "lead")
	lab.report = Message{ID: NewID(), From: "api", FromEpoch: lab.run, To: "lead", ToEpoch: lab.lead.epoch(), Kind: Finished, Text: "R", CreatedAt: time.Now()}
	lab.letters = countLetters(lab.dir)
	t.Cleanup(func() { testAccess.Delete(lab.dir) })
	if err := WriteJournal(lab.dir, "api", "end", TurnJournal{Epoch: lab.run, Op: "end", Ended: 100, Reports: []Message{lab.report}}); err != nil {
		t.Fatal(err)
	}
	return lab
}

// landRead lands report by the code a barrier runs, with no save after it,
// and has its recipient read it long enough ago for the sweep to retire it.
func (l raceLab) landRead(t *testing.T, report Message) {
	t.Helper()
	if err := withLock(l.dir, report.From, func() error {
		_, err := live(l.dir).publishReport(context.Background(), report.From, report)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	read := filepath.Join(state.DonePath(l.dir, report.To), report.ID+".json")
	if err := state.EnsureSubdir(state.DonePath(l.dir, report.To)); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(state.InboxPath(l.dir, report.To), report.ID+".json"), read); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * keepFinished)
	if err := os.Chtimes(read, old, old); err != nil {
		t.Fatal(err)
	}
}

// nextRunSweeps ends r, starts lead's next run and its sweep, and answers
// when the sweep has finished.
func (l raceLab) nextRunSweeps(t *testing.T) <-chan struct{} {
	t.Helper()
	l.lead.end()
	next := startRun(t, l.dir, "lead")
	done := make(chan struct{})
	go func() {
		defer close(done)
		(&Server{Dir: l.dir, Name: "lead", Epoch: next.epoch()}).sweepFinished()
	}()
	return done
}

// proofGone says the sweep retired the letter and r's marks.
func (l raceLab) proofGone(t *testing.T) bool {
	t.Helper()
	_, letter := os.Stat(filepath.Join(state.DonePath(l.dir, "lead"), l.report.ID+".json"))
	_, marks := os.Stat(filepath.Join(state.InboxPath(l.dir, "lead"), "once", l.lead.epoch()))
	return os.IsNotExist(letter) && os.IsNotExist(marks)
}

// atStep runs fn, once, when a publication of R reaches step.
func atStep(t *testing.T, step string, id string, fn func()) {
	t.Helper()
	var once sync.Once
	PublicationStep = func(at string, message Message) {
		if at == step && message.ID == id {
			once.Do(fn)
		}
	}
	t.Cleanup(func() { PublicationStep = nil })
}

// lockWaits answers once somebody finds the lock of name's mailbox held.
func lockWaits(t *testing.T, dir, name string) <-chan struct{} {
	t.Helper()
	waiting, mailbox := make(chan struct{}), state.InboxPath(dir, name)
	var once sync.Once
	state.LockWait = func(path string) {
		if path == mailbox {
			once.Do(func() { close(waiting) })
		}
	}
	t.Cleanup(func() { state.LockWait = nil })
	return waiting
}

const raceBound = 10 * time.Second

// The sweep waits for an attempt's evidence: a retry that read r live inside
// its section is at its first evidence read when r ends and the next run's
// sweep starts. The sweep waits until the section ends, and the retry finds
// the letter and writes nothing.
func TestTheSweepWaitsForAnAttemptsEvidence(t *testing.T) {
	lab := newRaceLab(t)
	waiting := lockWaits(t, lab.dir, "lead")
	var swept <-chan struct{}
	atStep(t, PublicationEvidence, lab.report.ID, func() {
		swept = lab.nextRunSweeps(t)
		select {
		case <-waiting:
		case <-swept:
			t.Error("the sweep ran inside the publisher's section")
		case <-time.After(raceBound):
			t.Error("the sweep neither waited nor finished")
		}
	})
	if err := lab.reconcile(t); err != nil {
		t.Fatal(err)
	}
	<-swept
	if writes := lab.letters.of(lab.report.ID); writes != 1 {
		t.Fatalf("R was written %d times", writes)
	}
	if !lab.proofGone(t) {
		t.Fatal("the sweep after the section retired nothing")
	}
}

// Admission is inside the section: a retry held at its successful liveness
// read, while r ends and the next run's sweep starts, holds the lock; the
// sweep waits, the retry finds the letter and writes nothing, and the sweep
// then retires the proof. With the liveness read before the lock, the held
// retry holds nothing: the sweep retires the proof, and the retry writes R
// again.
func TestAdmissionIsInsideTheSection(t *testing.T) {
	lab := newRaceLab(t)
	waiting := lockWaits(t, lab.dir, "lead")
	var swept <-chan struct{}
	atStep(t, PublicationAdmitted, lab.report.ID, func() {
		swept = lab.nextRunSweeps(t)
		select {
		case <-waiting:
		case <-swept:
		case <-time.After(raceBound):
			t.Error("the sweep neither waited nor finished")
		}
	})
	if err := lab.reconcile(t); err != nil {
		t.Fatal(err)
	}
	<-swept
	if writes := lab.letters.of(lab.report.ID); writes != 1 {
		t.Fatalf("R was written %d times", writes)
	}
	if !lab.proofGone(t) {
		t.Fatal("the sweep after the section retired nothing")
	}
}

// journalOf reads api's journal of the end, unfinished or done.
func (l raceLab) journalOf(t *testing.T) (TurnJournal, bool) {
	t.Helper()
	path := filepath.Join(JournalPath(l.dir, "api"), "end")
	journal, err := readJournalFile(path)
	if os.IsNotExist(err) {
		journal, err = readJournalFile(path + doneSuffix)
		if err != nil {
			t.Fatal(err)
		}
		return journal, true
	}
	if err != nil {
		t.Fatal(err)
	}
	return journal, journal.Done
}

// failJournalWrite fails the n-th write of api's journal of the end from
// now, as a sender stopped before it: the writes before it stay durable.
func (l raceLab) failJournalWrite(n int) {
	l.letters.mu.Lock()
	defer l.letters.mu.Unlock()
	l.letters.failAt, l.letters.failOn, l.letters.seen = filepath.Join(JournalPath(l.dir, "api"), "end"), n, 0
}

// A saved Published stays: a retry that saved it and stopped before the
// journal's done rewrite leaves a later barrier, after the sweep retired the
// proof, nothing to read about R. It reads no liveness for R, records no moot
// and writes no letter.
func TestASavedPublishedStays(t *testing.T) {
	lab := newRaceLab(t)
	var swept <-chan struct{}
	atStep(t, PublicationEvidence, lab.report.ID, func() { swept = lab.nextRunSweeps(t) })
	lab.failJournalWrite(2)
	if lab.reconcile(t) == nil {
		t.Fatal("the done rewrite was not stopped")
	}
	<-swept
	journal, done := lab.journalOf(t)
	if done || !slices.Contains(journal.Published, lab.report.ID) {
		t.Fatalf("the saved Published is not on record: %+v", journal)
	}
	if !lab.proofGone(t) {
		t.Fatal("the sweep retired nothing")
	}
	steps, since := 0, lab.letters.journalWrites()
	PublicationStep = func(string, Message) { steps++ }
	if err := lab.reconcile(t); err != nil {
		t.Fatal(err)
	}
	if _, done = lab.journalOf(t); steps != 0 || lab.letters.mooted(lab.report.ID, since) || !done {
		t.Fatalf("the later barrier looked at R again: %d steps, done %v", steps, done)
	}
	if writes := lab.letters.of(lab.report.ID); writes != 1 {
		t.Fatalf("R was written %d times", writes)
	}
}

// A crash before the save: the admitted retry's section ends and the sender
// stops before saving Published; the sweep retires the proof. A retry of the
// still unfinished journal finds r ended and records R moot, writing nothing.
func TestACrashBeforeTheSaveLeavesTheReportMoot(t *testing.T) {
	lab := newRaceLab(t)
	var swept <-chan struct{}
	atStep(t, PublicationAdmitted, lab.report.ID, func() { swept = lab.nextRunSweeps(t) })
	lab.failJournalWrite(1)
	if lab.reconcile(t) == nil {
		t.Fatal("the save was not stopped")
	}
	<-swept
	if !lab.proofGone(t) {
		t.Fatal("the sweep retired nothing")
	}
	lab.failJournalWrite(0)
	since := lab.letters.journalWrites()
	if err := lab.reconcile(t); err != nil {
		t.Fatal(err)
	}
	if _, done := lab.journalOf(t); !done || !lab.letters.mooted(lab.report.ID, since) {
		t.Fatalf("R was not recorded moot through the journal: done %v", done)
	}
	if writes := lab.letters.of(lab.report.ID); writes != 1 {
		t.Fatalf("R was written %d times", writes)
	}
}
