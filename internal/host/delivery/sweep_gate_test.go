package delivery

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/mail/mailtest"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// gateLab is the race lab's setup built on mail's exported operations, for
// the tests of the server's own gate around the sweep — the live-run read and
// the lock (docs/rules/effects.md#publication-once): a letter R of api's, published
// once, has landed in lead's run r under its marks, r has read it, and its age
// has passed the sweep's cutoff.
type gateLab struct {
	twoSessionLab
	lead   *endableRun
	report mail.Message
}

func newGateLab(t *testing.T) gateLab {
	t.Helper()
	lab := gateLab{twoSessionLab: newTwoSessionLab(t)}
	// The lab's lead is served by this process, which never ends: r is a run
	// of its own here.
	if err := registry.Remove(lab.dir, "lead"); err != nil {
		t.Fatal(err)
	}
	lab.lead = startRun(t, lab.dir, "lead")
	lab.report = mail.Message{ID: mail.NewID(), From: "api", FromEpoch: lab.run, To: "lead", ToEpoch: lab.lead.epoch(), Kind: mail.Finished, Text: "R", CreatedAt: time.Now()}
	lab.landRead(t, lab.report)
	return lab
}

// landRead publishes report once and has its recipient read it long enough
// ago for the sweep to retire it.
func (l gateLab) landRead(t *testing.T, report mail.Message) {
	t.Helper()
	if _, err := mail.PublishOnce(t.Context(), l.dir, report, nil); err != nil {
		t.Fatal(err)
	}
	read := filepath.Join(state.DonePath(l.dir, report.To), report.ID+".json")
	if err := state.EnsureSubdir(state.DonePath(l.dir, report.To)); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(state.InboxPath(l.dir, report.To), report.ID+".json"), read); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-mailtest.PastRetention)
	if err := os.Chtimes(read, old, old); err != nil {
		t.Fatal(err)
	}
}

// proofGone says the sweep retired the letter and r's marks.
func (l gateLab) proofGone(t *testing.T) bool {
	t.Helper()
	_, letter := os.Stat(filepath.Join(state.DonePath(l.dir, "lead"), l.report.ID+".json"))
	_, marks := os.Stat(filepath.Join(state.InboxPath(l.dir, "lead"), "once", l.lead.epoch()))
	return os.IsNotExist(letter) && os.IsNotExist(marks)
}

// A stale sweeper: the server of r, which has ended, sweeps after the next
// run is live. It retires nothing at all, and the live run's marks and
// letters stay.
func TestAStaleSweeperRetiresNothing(t *testing.T) {
	lab := newGateLab(t)
	lab.lead.end()
	next := startRun(t, lab.dir, "lead")
	second := mail.Message{ID: mail.NewID(), From: "api", FromEpoch: lab.run, To: "lead", ToEpoch: next.epoch(), Kind: mail.Finished, Text: "R2", CreatedAt: time.Now()}
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
	lab := newGateLab(t)
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
