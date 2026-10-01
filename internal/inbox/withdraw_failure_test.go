package inbox

import (
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// readOnly makes a mailbox directory refuse new files until the test ends, as
// a full disk or a lost permission would.
func readOnly(t *testing.T, directory string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root writes into a read-only directory")
	}
	if err := state.EnsureSubdir(directory); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(directory, 0o700) })
}

func writable(t *testing.T, directory string) {
	t.Helper()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
}

// announcedTask is a task whose notice went out: readable in unread/, still
// waiting in inbox/ until the server settles it.
func announcedTask(t *testing.T) *heldFixture {
	t.Helper()
	f := startDelivering(t, Task)
	f.announced(t)
	f.until(t, Delivered)
	return f
}

// A withdrawal that wrote its status and then failed leaves the original text
// readable by hard link. The reader must still find it withdrawn and owe
// nothing, and whatever reads the status must go on seeing withdrawn.
func TestAHalfDoneWithdrawalIsNeverReadAsTheTask(t *testing.T) {
	f := announcedTask(t)
	unread := state.UnreadPath(f.dir, "api")
	readOnly(t, unread)
	if _, err := withdrawNow(t, f.dir, f.task, nil); err == nil {
		t.Fatal("the tombstone could not be written, and withdraw said nothing")
	}
	writable(t, unread)
	read := readAll(t, f.dir, "api-epoch")
	if len(read) != 1 || read[0].Withdrawn == nil || read[0].Text == f.task.Text {
		t.Fatalf("the reader got %+v", read)
	}
	if status, _, _ := ReadStatus(f.dir, "api", f.task.ID); status.State != Failed || !status.Withdrawn {
		t.Fatalf("reading turned the withdrawn status into %+v", status)
	}
	if waiters := Waiters(f.dir, "api", "api-epoch"); len(waiters) != 0 {
		t.Fatalf("a withdrawn task was owed a report: %+v", waiters)
	}
	// What is kept is what was read: an archive holding the original would
	// show the task to whatever reads done/ later as if it had been read.
	if !must(tombstoneIn(state.DonePath(f.dir, "api"), f.task.ID)) || must(isIn(state.UnreadPath(f.dir, "api"), f.task.ID)) {
		t.Fatal("done/ keeps the original, not the tombstone that was read")
	}
}

// The next withdraw finishes what the failed one began, rather than answering
// that it was done already.
func TestAWithdrawalRetriedFinishesTheSteps(t *testing.T) {
	f := announcedTask(t)
	unread := state.UnreadPath(f.dir, "api")
	readOnly(t, unread)
	if _, err := withdrawNow(t, f.dir, f.task, nil); err == nil {
		t.Fatal("the tombstone could not be written, and withdraw said nothing")
	}
	writable(t, unread)
	if result, err := withdrawNow(t, f.dir, f.task, nil); err != nil || result != WithdrawnAnnounced {
		t.Fatalf("the retry: %v %v", result, err)
	}
	f.stillWithdrawn(t)
	if result, err := withdrawNow(t, f.dir, f.task, nil); err != nil || result != AlreadyWithdrawn {
		t.Fatalf("once finished: %v %v", result, err)
	}
}

// An edit whose replacement cannot be written changes nothing: the old
// message is not withdrawn without something in its place.
func TestAnEditThatCannotWriteItsReplacementChangesNothing(t *testing.T) {
	f := announcedTask(t)
	putMessage = func(string, Message) error { return errors.New("disk full") }
	t.Cleanup(func() { putMessage = Put })
	replacement := message("rerun the smoke on staging")
	replacement.Kind, replacement.FromEpoch, replacement.ToEpoch, replacement.Replaces = Task, "web-epoch", "api-epoch", f.task.ID
	if _, err := withdrawNow(t, f.dir, f.task, &replacement); err == nil {
		t.Fatal("the replacement was not written, and edit said nothing")
	}
	if status, _, _ := ReadStatus(f.dir, "api", f.task.ID); status.Withdrawn || status.State != Delivered {
		t.Fatalf("the old task became %+v", status)
	}
	if stone := tombstone(t, f.dir, f.task.ID); stone == nil || stone.Withdrawn != nil {
		t.Fatalf("the old task is not readable as it was: %+v", stone)
	}
}

// An edit whose withdrawal fails after the replacement was written takes the
// replacement back, and a second edit finishes with its own.
func TestAnEditThatCannotWithdrawTakesItsReplacementBack(t *testing.T) {
	f := announcedTask(t)
	unread := state.UnreadPath(f.dir, "api")
	readOnly(t, unread)
	first := message("rerun the smoke on staging")
	first.Kind, first.FromEpoch, first.ToEpoch, first.Replaces = Task, "web-epoch", "api-epoch", f.task.ID
	if _, err := withdrawNow(t, f.dir, f.task, &first); err == nil {
		t.Fatal("the tombstone could not be written, and edit said nothing")
	}
	writable(t, unread)
	if _, err := os.Stat(filepath.Join(state.InboxPath(f.dir, "api"), first.ID+".json")); !os.IsNotExist(err) {
		t.Fatalf("the replacement of a failed edit stayed to be announced: %v", err)
	}
	second := message("rerun the smoke on staging, then lint")
	second.Kind, second.FromEpoch, second.ToEpoch, second.Replaces = Task, "web-epoch", "api-epoch", f.task.ID
	if result, err := withdrawNow(t, f.dir, f.task, &second); err != nil || result != WithdrawnAnnounced {
		t.Fatalf("the second edit: %v %v", result, err)
	}
	if stone := tombstone(t, f.dir, f.task.ID); stone == nil || stone.Withdrawn == nil || stone.Withdrawn.ReplacedBy != second.ID {
		t.Fatalf("tombstone %+v", stone)
	}
}

// The waiting copy is not a step of the withdrawal: with the tombstone in
// place, one that would not go leaves a finished withdrawal, which a retry
// answers as done — withdraw and edit alike — and the serving process clears
// the copy as it settles the status.
func TestAWaitingCopyThatWillNotGoLeavesTheWithdrawalFinished(t *testing.T) {
	// Set before the server starts, which reads it; switched through the flag.
	var stuck atomic.Bool
	removeWaiting = func(path string) error {
		if stuck.Load() {
			return os.ErrPermission
		}
		return os.Remove(path)
	}
	t.Cleanup(func() { removeWaiting = os.Remove })
	f := startHeld(t, Task, false)
	id := f.announced(t)
	f.until(t, Held)
	stuck.Store(true)
	if result, err := withdrawNow(t, f.dir, f.task, nil); err != nil || result != WithdrawnHeld {
		t.Fatalf("withdraw: %v %v", result, err)
	}
	waiting := filepath.Join(state.InboxPath(f.dir, "api"), f.task.ID+".json")
	if _, err := os.Stat(waiting); err != nil {
		t.Fatalf("the seam did not keep the waiting copy: %v", err)
	}
	if result, err := withdrawNow(t, f.dir, f.task, nil); err != nil || result != AlreadyWithdrawn {
		t.Fatalf("the retry: %v %v", result, err)
	}
	replacement := message("rerun the smoke on staging")
	replacement.Kind, replacement.FromEpoch, replacement.ToEpoch, replacement.Replaces = Task, "web-epoch", "api-epoch", f.task.ID
	if _, err := withdrawNow(t, f.dir, f.task, &replacement); !errors.Is(err, ErrAlreadyWithdrawn) {
		t.Fatalf("an edit after it: %v", err)
	}
	if must(isIn(state.InboxPath(f.dir, "api"), replacement.ID)) {
		t.Fatal("a refused edit left its replacement to be announced")
	}
	stuck.Store(false)
	f.receipts <- Receipt{ID: id, Result: Result{State: Delivered, Via: "socket"}}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(waiting); os.IsNotExist(err) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the serving process never cleared the waiting copy")
		}
		time.Sleep(20 * time.Millisecond)
	}
	f.stillWithdrawn(t)
}
