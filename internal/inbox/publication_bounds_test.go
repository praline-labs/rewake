package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// A first landing across run end: with no earlier landing, r ends inside the
// held section after its liveness read. Absence still proves no landing, so
// R is written once, to r, and saved Published; nothing checks the run again
// and nothing writes it again.
func TestAFirstLandingAcrossRunEndIsWrittenOnce(t *testing.T) {
	lab := newBareRaceLab(t)
	atStep(t, PublicationAdmitted, lab.report.ID, lab.lead.end)
	if err := lab.reconcile(t); err != nil {
		t.Fatal(err)
	}
	if writes := lab.letters.of(lab.report.ID); writes != 1 {
		t.Fatalf("R was written %d times", writes)
	}
	raw, err := os.ReadFile(filepath.Join(state.InboxPath(lab.dir, "lead"), lab.report.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var letter Message
	if err := json.Unmarshal(raw, &letter); err != nil || letter.ToEpoch != lab.lead.epoch() {
		t.Fatalf("the letter is not addressed to r: %+v %v", letter, err)
	}
	published := lab.letters.recorded(lab.report.ID, 0, func(j TurnJournal) []string { return j.Published })
	if _, done := lab.journalOf(t); !published || !done {
		t.Fatalf("the journal did not save Published: saved %v, done %v", published, done)
	}
	if err := lab.reconcile(t); err != nil {
		t.Fatal(err)
	}
	if writes := lab.letters.of(lab.report.ID); writes != 1 {
		t.Fatalf("R was written %d times", writes)
	}
}

// A stale sweeper: the server of r, which has ended, sweeps after the next
// run is live. It retires nothing at all, and the live run's marks and
// letters stay.
func TestAStaleSweeperRetiresNothing(t *testing.T) {
	lab := newRaceLab(t)
	lab.lead.end()
	next := startRun(t, lab.dir, "lead")
	second := Message{ID: NewID(), From: "api", FromEpoch: lab.run, To: "lead", ToEpoch: next.epoch(), Kind: Finished, Text: "R2", CreatedAt: time.Now()}
	lab.landRead(t, second)
	(&Server{Dir: lab.dir, Name: "lead", Epoch: lab.lead.epoch()}).sweepFinished()
	for _, path := range []string{
		filepath.Join(state.DonePath(lab.dir, "lead"), second.ID+".json"),
		filepath.Join(state.InboxPath(lab.dir, "lead"), "once", next.epoch()),
		filepath.Join(state.DonePath(lab.dir, "lead"), lab.report.ID+".json"),
		filepath.Join(state.InboxPath(lab.dir, "lead"), "once", lab.lead.epoch()),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("the stale sweeper retired %s: %v", path, err)
		}
	}
}

// An unusable lock: a sweep that cannot take the mailbox lock retires no
// letter and no mark. Once the lock is usable again, the same sweep does.
func TestASweepWithoutTheLockRetiresNothing(t *testing.T) {
	lab := newRaceLab(t)
	lab.lead.end()
	next := startRun(t, lab.dir, "lead")
	lock := filepath.Join(state.InboxPath(lab.dir, "lead"), ".lock")
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(lock, 0o700); err != nil {
		t.Fatal(err)
	}
	sweeper := &Server{Dir: lab.dir, Name: "lead", Epoch: next.epoch()}
	sweeper.sweepFinished()
	if lab.proofGone(t) {
		t.Fatal("the sweep retired the proof without the lock")
	}
	for _, path := range []string{
		filepath.Join(state.DonePath(lab.dir, "lead"), lab.report.ID+".json"),
		filepath.Join(state.InboxPath(lab.dir, "lead"), "once", lab.lead.epoch()),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("the sweep without the lock retired %s: %v", path, err)
		}
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	sweeper.sweepFinished()
	if !lab.proofGone(t) {
		t.Fatal("with the lock usable the sweep retired nothing: the case proves nothing")
	}
}

// shortWait bounds the wait for a recipient's lock for the test.
func shortWait(t *testing.T, wait time.Duration) {
	t.Helper()
	before := recipientLockWait
	recipientLockWait = wait
	t.Cleanup(func() { recipientLockWait = before })
}

// A busy recipient: the wait for its lock expires. The barrier writes
// nothing, records no stop and leaves the journal unfinished; the next
// barrier publishes once.
func TestABusyRecipientFailsTheTurnWithoutAStop(t *testing.T) {
	lab := newBareRaceLab(t)
	shortWait(t, 50*time.Millisecond)
	held, release, released := make(chan struct{}), make(chan struct{}), make(chan error)
	go func() {
		released <- withLock(lab.dir, "lead", func() error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	err := lab.reconcile(t)
	close(release)
	if err := <-released; err != nil {
		t.Fatal(err)
	}
	if !errors.Is(err, state.ErrMailboxBusy) {
		t.Fatalf("the barrier answers %v, not a busy recipient", err)
	}
	if writes := lab.letters.of(lab.report.ID); writes != 0 {
		t.Fatalf("R was written %d times past a busy lock", writes)
	}
	if recorded, err := recordedStop(lab.dir, "api"); recorded != nil || err != nil {
		t.Fatalf("a busy recipient recorded a stop: %+v %v", recorded, err)
	}
	if err := MailboxStopped(lab.dir, "api"); err != nil {
		t.Fatalf("a busy recipient stopped the mailbox: %v", err)
	}
	if journal, done := lab.journalOf(t); done || len(journal.Published) > 0 || len(journal.Moot) > 0 {
		t.Fatalf("the journal moved past a busy recipient: %+v", journal)
	}
	if err := lab.reconcile(t); err != nil {
		t.Fatal(err)
	}
	if _, done := lab.journalOf(t); lab.letters.of(lab.report.ID) != 1 || !done {
		t.Fatalf("the next barrier did not publish R once: %d writes", lab.letters.of(lab.report.ID))
	}
}

// Two mailboxes publishing to each other at once, each inside its own
// barrier: both hold their own lock before either asks for the other's. Both
// finish within their bounds or answer busy, and each report lands once.
func TestTwoMailboxesPublishingToEachOtherDoNotDeadlock(t *testing.T) {
	dir := stateDir(t)
	letters := countLetters(dir)
	t.Cleanup(func() { testAccess.Delete(dir) })
	shortWait(t, 200*time.Millisecond)
	runs := map[string]*endableRun{"east": startRun(t, dir, "east"), "west": startRun(t, dir, "west")}
	reports := map[string]Message{}
	for name, other := range map[string]string{"east": "west", "west": "east"} {
		report := Message{ID: NewID(), From: name, FromEpoch: runs[name].epoch(), To: other, ToEpoch: runs[other].epoch(), Kind: Finished, Text: "R", CreatedAt: time.Now()}
		reports[name] = report
		if err := WriteJournal(dir, name, "end", TurnJournal{Epoch: runs[name].epoch(), Op: "end", Ended: 100, Reports: []Message{report}}); err != nil {
			t.Fatal(err)
		}
	}
	arrived, both := make(chan struct{}, 2), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { afterReading = func() {} })
	afterReading = func() {
		arrived <- struct{}{}
		if len(arrived) == 2 {
			once.Do(func() { close(both) })
		}
		select {
		case <-both:
		case <-time.After(raceBound):
		}
	}
	answers := make(chan error, 2)
	for name := range runs {
		go func() {
			answers <- withLock(dir, name, func() error { return Reconcile(context.Background(), dir, name) })
		}()
	}
	for range 2 {
		select {
		case err := <-answers:
			if err != nil && !errors.Is(err, state.ErrMailboxBusy) {
				t.Errorf("a barrier answers %v", err)
			}
		case <-time.After(raceBound):
			t.Fatal("two barriers publishing to each other did not finish: a deadlock")
		}
	}
	afterReading = func() {}
	for name, report := range reports {
		if err := withLock(dir, name, func() error { return Reconcile(context.Background(), dir, name) }); err != nil {
			t.Fatalf("the retry of %s answers %v", name, err)
		}
		if writes := letters.of(report.ID); writes != 1 {
			t.Errorf("the report of %s was written %d times", name, writes)
		}
	}
}

// A failed main's report to itself completes inside its barrier's own lock:
// a second acquisition would wait out its bound against itself, so the bound
// here is far past the test's deadline. Kept locally, or answering a task.
func TestAReportToItselfIsPublishedInsideItsOwnLock(t *testing.T) {
	shortWait(t, 10*raceBound)
	for _, answering := range []bool{false, true} {
		dir := stateDir(t)
		letters := countLetters(dir)
		t.Cleanup(func() { testAccess.Delete(dir) })
		run := startRun(t, dir, "solo")
		report := Message{ID: NewID(), From: "solo", FromEpoch: run.epoch(), To: "solo", ToEpoch: run.epoch(), Kind: Error, Text: "R", CreatedAt: time.Now()}
		if answering {
			report.InReplyTo = []string{NewID()}
		}
		if err := WriteJournal(dir, "solo", "end", TurnJournal{Epoch: run.epoch(), Op: "end", Ended: 100, Reports: []Message{report}}); err != nil {
			t.Fatal(err)
		}
		answer := make(chan error, 1)
		go func() {
			answer <- withLock(dir, "solo", func() error { return Reconcile(context.Background(), dir, "solo") })
		}()
		select {
		case err := <-answer:
			if err != nil {
				t.Fatalf("answering %v: %v", answering, err)
			}
		case <-time.After(raceBound):
			t.Fatalf("answering %v: the report to itself waited for its own lock", answering)
		}
		if writes := letters.of(report.ID); writes != 1 {
			t.Fatalf("answering %v: the report to itself was written %d times", answering, writes)
		}
	}
}

// Inside the recipient's lock the run is read without the name lock: a
// lookup that cleans up after an ended run takes the name's lock, which a
// holder of a mailbox lock must not enter. So a publication to a run that has
// ended, a report or a heads-up, leaves that run's record where it is.
func TestAPublicationReadsTheRunWithoutCleaningUp(t *testing.T) {
	lab := newBareRaceLab(t)
	lab.lead.end()
	record := state.SessionPath(lab.dir, "lead")
	heads := Message{ID: NewID(), From: "api", FromEpoch: lab.run, To: "lead", ToEpoch: lab.lead.epoch(), Kind: Note, Text: "H", CreatedAt: time.Now()}
	if _, err := PublishOnce(context.Background(), lab.dir, heads, nil); !errors.Is(err, ErrRecipientEnded) {
		t.Fatalf("a heads-up to an ended run: %v", err)
	}
	if _, err := os.Stat(record); err != nil {
		t.Fatalf("the heads-up's look at the run removed its record: %v", err)
	}
	if err := lab.reconcile(t); err != nil {
		t.Fatal(err)
	}
	if !lab.letters.mooted(lab.report.ID, 0) {
		t.Fatal("the report to an ended run was not recorded moot")
	}
	if _, err := os.Stat(record); err != nil {
		t.Fatalf("the report's look at the run removed its record: %v", err)
	}
}
