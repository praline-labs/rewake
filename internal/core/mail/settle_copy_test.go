package mail

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// blockDone puts a file where name's done/ belongs, so every archive into it
// fails, and answers the call that takes the block away.
func blockDone(t *testing.T, dir, name string) func() {
	t.Helper()
	blocked := state.DonePath(dir, name)
	writeRaw(t, blocked, "not a directory")
	unblocked := false
	unblock := func() {
		if unblocked {
			return
		}
		unblocked = true
		if err := os.Remove(blocked); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(unblock)
	return unblock
}

// landUnread writes report as a letter of its recipient that has left the
// waiting set and is readable in unread/ only: its notice went out and the
// waiting copy was settled.
func landUnread(t *testing.T, dir string, report Message) string {
	t.Helper()
	if err := live(dir).put(report); err != nil {
		t.Fatal(err)
	}
	if err := state.EnsureSubdir(state.UnreadPath(dir, report.To)); err != nil {
		t.Fatal(err)
	}
	unread := filepath.Join(state.UnreadPath(dir, report.To), report.ID+".json")
	if err := os.Rename(filepath.Join(state.InboxPath(dir, report.To), report.ID+".json"), unread); err != nil {
		t.Fatal(err)
	}
	return unread
}

// A refused letter whose waiting copy is gone has its readable copy as the
// last; an archive into done/ that fails leaves that copy where it is, and
// the next settling archives it (docs/rules/effects.md#retiring-a-letter).
func TestAFailedArchiveKeepsTheLastCopy(t *testing.T) {
	lab := newTwoSessionLab(t)
	report := lab.report(Finished)
	unread := landUnread(t, lab.dir, report)
	unblock := blockDone(t, lab.dir, report.To)
	settleLocked := func() {
		if err := withLock(lab.dir, report.To, func() error {
			SettleLocked(lab.dir, report.To, report.ID, Failed)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	settleLocked()
	if _, err := os.Stat(unread); err != nil {
		t.Fatalf("the last copy went when its archive failed: %v", err)
	}
	unblock()
	settleLocked()
	if _, err := os.Stat(filepath.Join(state.DonePath(lab.dir, report.To), report.ID+".json")); err != nil {
		t.Fatalf("the next settling did not archive the copy: %v", err)
	}
	if _, err := os.Stat(unread); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the archived copy is still unread: %v", err)
	}
}

// A copy that is not the last goes; one that is the last stays, whatever the
// outcome: a delivered letter whose readable copy is gone keeps its waiting one.
func TestSettlingADeliveredLetterKeepsItsOnlyCopy(t *testing.T) {
	lab := newTwoSessionLab(t)
	report := lab.report(Finished)
	if err := live(lab.dir).put(report); err != nil {
		t.Fatal(err)
	}
	waiting := filepath.Join(state.InboxPath(lab.dir, report.To), report.ID+".json")
	SettleLocked(lab.dir, report.To, report.ID, Delivered)
	if _, err := os.Stat(waiting); err != nil {
		t.Fatalf("the only copy of a delivered letter went: %v", err)
	}
	if err := LinkUnread(lab.dir, report.To, report.ID); err != nil {
		t.Fatal(err)
	}
	SettleLocked(lab.dir, report.To, report.ID, Delivered)
	if _, err := os.Stat(waiting); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the waiting copy stayed beside its readable one: %v", err)
	}
}

// Only a regular file at a stage path is a copy of the letter. A directory
// where a refused letter's archive belongs makes the archive fail, and the
// readable copy stays as the last; the obstruction gone, the next settling
// archives it.
func TestADirectoryAtTheArchiveIsNoCopy(t *testing.T) {
	lab := newTwoSessionLab(t)
	report := lab.report(Finished)
	unread := landUnread(t, lab.dir, report)
	target := filepath.Join(state.DonePath(lab.dir, report.To), report.ID+".json")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	settleLocked := func() {
		if err := withLock(lab.dir, report.To, func() error {
			SettleLocked(lab.dir, report.To, report.ID, Failed)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	settleLocked()
	if _, err := os.Stat(unread); err != nil {
		t.Fatalf("an archive onto a directory took the last copy: %v", err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	settleLocked()
	if info, err := os.Lstat(target); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("the next settling did not archive the copy: %v", err)
	}
}
