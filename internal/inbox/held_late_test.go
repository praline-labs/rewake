package inbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

func startDelivering(t *testing.T, kind Kind) *heldFixture {
	t.Helper()
	return startAnswering(t, kind, false, Result{State: Delivered, Via: "socket"})
}

// A hold reported after the delivery was counted takes it back: the sender
// sees held again, not delivered, and the hold then ends like any other.
func TestALateHoldTakesTheDeliveryBack(t *testing.T) {
	f := startDelivering(t, Task)
	id := f.announced(t)
	f.until(t, Delivered)
	f.receipts <- Receipt{ID: id, Result: Result{State: Held, Via: "socket", Detail: "held late"}}
	if status := f.until(t, Held); status.Detail != "held late" {
		t.Fatalf("got %+v", status)
	}
	f.receipts <- Receipt{ID: id, Result: Result{State: Failed, Via: "socket", Detail: "it expired unreleased"}}
	f.until(t, Failed)
	if f.told(t) == nil {
		t.Fatal("the sender, who may have gone on as delivered, was not told")
	}
	if _, err := os.Stat(filepath.Join(state.UnreadPath(f.dir, "api"), f.task.ID+".json")); !os.IsNotExist(err) {
		t.Fatalf("a failed message stayed readable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(state.DonePath(f.dir, "api"), f.task.ID+".json")); err != nil {
		t.Fatalf("a failed message was not archived: %v", err)
	}
}

// A refusal or expiry with no hold reported first means the line never
// arrived: failed, and the sender is told.
func TestALateFailureWithoutAHoldFails(t *testing.T) {
	f := startDelivering(t, Task)
	id := f.announced(t)
	f.until(t, Delivered)
	f.receipts <- Receipt{ID: id, Result: Result{State: Failed, Via: "socket", Detail: "refused"}}
	f.until(t, Failed)
	if note := f.told(t); note == nil || !strings.Contains(note.Text, "refused") {
		t.Fatalf("the sender was not told: %+v", note)
	}
}

// What the agent has read stays read, whatever is said late.
func TestALateHoldLeavesAReadMessageRead(t *testing.T) {
	f := startDelivering(t, Task)
	id := f.announced(t)
	f.until(t, Delivered)
	readAll(t, f.dir, "api-epoch")
	f.until(t, Read)
	f.receipts <- Receipt{ID: id, Result: Result{State: Held, Detail: "held late"}}
	f.receipts <- Receipt{ID: id, Result: Result{State: Failed, Detail: "expired"}}
	time.Sleep(100 * time.Millisecond)
	if status, _ := ReadStatus(f.dir, "api", f.task.ID); status.State != Read {
		t.Fatalf("a read task became %+v", status)
	}
	if f.undelivered(t) != nil {
		t.Fatal("the sender of a task that was read was told it failed")
	}
}

// A session ending fails what it took back as well, though its waiting copy
// went with the delivery.
func TestASessionEndingFailsALateHold(t *testing.T) {
	f := startDelivering(t, Task)
	id := f.announced(t)
	f.until(t, Delivered)
	f.receipts <- Receipt{ID: id, Result: Result{State: Held, Detail: "held late"}}
	f.until(t, Held)
	f.stop()
	if status, _ := ReadStatus(f.dir, "api", f.task.ID); status.State != Failed {
		t.Fatalf("a hold nothing can release now stayed %+v", status)
	}
	if f.told(t) == nil {
		t.Fatal("the sender was not told")
	}
}

// A word about a delivery older than the window changes nothing.
func TestAWordPastTheWindowIsIgnored(t *testing.T) {
	f := startDelivering(t, Task)
	id := f.announced(t)
	f.until(t, Delivered)
	f.stop()
	recent := f.server.recent[id]
	recent.at = time.Now().Add(-2 * LateWordWindow)
	f.server.recent[id] = recent
	f.server.receive(Receipt{ID: id, Result: Result{State: Held, Detail: "held late"}})
	if status, _ := ReadStatus(f.dir, "api", f.task.ID); status.State != Delivered {
		t.Fatalf("a word past the window changed the status to %+v", status)
	}
}

// A wrapper killed while its session held a message never says how the hold
// ended. The next session with the name fails it and tells its sender, who
// would otherwise wait for a report nobody owes — whether the waiting copy is
// still there or went with a delivery taken back.
func TestAHoldLeftByAKilledRunFailsWithTheNextOne(t *testing.T) {
	for _, taken := range []bool{false, true} {
		dir := stateDir(t)
		held := message("rerun the smoke")
		held.Kind, held.FromEpoch, held.ToEpoch = Task, "web-epoch", "dead-epoch"
		if err := Put(dir, held); err != nil {
			t.Fatal(err)
		}
		if err := linkUnread(dir, "api", held.ID); err != nil {
			t.Fatal(err)
		}
		if err := writeStatus(dir, "api", held.ID, Result{State: Held, Via: "socket", Detail: "held"}); err != nil {
			t.Fatal(err)
		}
		if taken {
			if err := os.Remove(filepath.Join(state.InboxPath(dir, "api"), held.ID+".json")); err != nil {
				t.Fatal(err)
			}
		}
		server := &Server{Dir: dir, Name: "api", Epoch: "api-epoch", Deliver: func(context.Context, Message) Result {
			t.Error("a message of the earlier run was delivered again")
			return Result{State: Delivered}
		}}
		serveUntil(t, server, func() bool {
			status, _ := ReadStatus(dir, "api", held.ID)
			return status.State == Failed
		})
		status, _ := ReadStatus(dir, "api", held.ID)
		if !strings.Contains(status.Detail, "held it and ended") {
			t.Fatalf("taken back %v: %+v", taken, status)
		}
		f := &heldFixture{dir: dir, task: held}
		if f.told(t) == nil {
			t.Fatalf("taken back %v: the sender was not told", taken)
		}
		for _, copy := range []string{state.InboxPath(dir, "api"), state.UnreadPath(dir, "api")} {
			if _, err := os.Stat(filepath.Join(copy, held.ID+".json")); !os.IsNotExist(err) {
				t.Fatalf("taken back %v: a copy stayed in %s: %v", taken, copy, err)
			}
		}
	}
}
