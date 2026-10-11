//go:build rewakefault

package mail

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// The waiting copy is not a step of the withdrawal: with the tombstone in
// place, one that would not go leaves a finished withdrawal, which a retry
// answers as done — withdraw and edit alike — and the serving process clears
// the copy as it settles the status.
func TestAWaitingCopyThatWillNotGoLeavesTheWithdrawalFinished(t *testing.T) {
	f := startHeld(t, Task, false)
	id := f.announced(t)
	f.until(t, Held)
	waiting := filepath.Join(state.InboxPath(f.dir, "api"), f.task.ID+".json")
	attempts, unstick := FailWaitingRemoval(waiting)
	t.Cleanup(unstick)
	if result, err := withdrawNow(t, f.dir, f.task, nil); err != nil || result != WithdrawnHeld {
		t.Fatalf("withdraw: %v %v", result, err)
	}
	if _, err := os.Stat(waiting); err != nil || attempts() == 0 {
		t.Fatalf("the failing removal did not keep the waiting copy: %d attempts, %v", attempts(), err)
	}
	if result, err := withdrawNow(t, f.dir, f.task, nil); err != nil || result != AlreadyWithdrawn {
		t.Fatalf("the retry: %v %v", result, err)
	}
	replacement := message("rerun the smoke on staging")
	replacement.Kind, replacement.FromEpoch, replacement.ToEpoch, replacement.Replaces = Task, "web-epoch", "api-epoch", f.task.ID
	if _, err := withdrawNow(t, f.dir, f.task, &replacement); !errors.Is(err, ErrAlreadyWithdrawn) {
		t.Fatalf("an edit after it: %v", err)
	}
	if must(IsIn(state.InboxPath(f.dir, "api"), replacement.ID)) {
		t.Fatal("a refused edit left its replacement to be announced")
	}
	unstick()
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
