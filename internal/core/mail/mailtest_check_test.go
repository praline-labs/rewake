package mail

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail/mailtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The fixtures tests outside mail build its records with are mail's records:
// each path is the one mail uses, each reading is mail's reading, and a wait
// written by Owe reads as markAwaiting's does.
func TestTheMailtestFixturesAreMailsRecords(t *testing.T) {
	dir := stateDir(t)
	if got, want := mailtest.StatusPath(dir, "api", "m1"), statusPath(dir, "api", "m1"); got != want {
		t.Errorf("StatusPath = %s, want %s", got, want)
	}
	if got, want := mailtest.AnswerReceiptsPath(dir, "api"), answerReceiptsPath(dir, "api"); got != want {
		t.Errorf("AnswerReceiptsPath = %s, want %s", got, want)
	}
	if mailtest.PastRetention != 2*keepFinished {
		t.Errorf("PastRetention = %v, want twice keepFinished, %v", mailtest.PastRetention, 2*keepFinished)
	}
	if mailtest.StaleAnswering != 2*answeringFresh {
		t.Errorf("StaleAnswering = %v, want twice answeringFresh, %v", mailtest.StaleAnswering, 2*answeringFresh)
	}

	for _, id := range []string{"absent", "pinned", "empty"} {
		if id != "absent" {
			thread := map[string]string{"pinned": "root-A", "empty": ""}[id]
			if err := mailtest.WriteDeliveryThread(dir, "api", id, thread); err != nil {
				t.Fatal(err)
			}
			if raw, err := os.ReadFile(filepath.Join(ThreadPath(dir, "api"), id)); err != nil || string(raw) != thread {
				t.Errorf("WriteDeliveryThread(%s) left %q, %v at ThreadPath", id, raw, err)
			}
		}
		got, err := mailtest.DeliveryThread(dir, "api", id)
		want, wantErr := deliveryThread(dir, "api", id)
		if got != want || (err == nil) != (wantErr == nil) {
			t.Errorf("DeliveryThread(%s) = %q, %v; deliveryThread = %q, %v", id, got, err, want, wantErr)
		}
	}

	task := message("rerun the smoke")
	task.Kind, task.FromEpoch, task.ToEpoch = Task, "web-epoch", "api-epoch"
	if err := Put(dir, task); err != nil {
		t.Fatal(err)
	}
	if err := LinkUnread(dir, "api", task.ID); err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{state.UnreadPath(dir, "api"), state.DonePath(dir, "api")} {
		got, err := mailtest.IsTombstone(directory, task.ID)
		want, wantErr := tombstoneIn(directory, task.ID)
		if got != want || err != nil || wantErr != nil {
			t.Errorf("before the withdrawal, IsTombstone(%s) = %v, %v; tombstoneIn = %v, %v", directory, got, err, want, wantErr)
		}
	}
	if _, err := withdrawNow(t, dir, task, nil); err != nil {
		t.Fatal(err)
	}
	stones := 0
	for _, directory := range []string{state.UnreadPath(dir, "api"), state.DonePath(dir, "api")} {
		got, err := mailtest.IsTombstone(directory, task.ID)
		want, wantErr := tombstoneIn(directory, task.ID)
		if got != want || err != nil || wantErr != nil {
			t.Errorf("after the withdrawal, IsTombstone(%s) = %v, %v; tombstoneIn = %v, %v", directory, got, err, want, wantErr)
		}
		if got {
			stones++
		}
	}
	if stones == 0 {
		t.Error("no tombstone was found: the comparison proves nothing")
	}

	before := time.Now().UnixNano()
	if err := mailtest.Owe(dir, "api", "5.5", "web", "40.4", "m0", "m1"); err != nil {
		t.Fatal(err)
	}
	other := stateDir(t)
	for _, id := range []string{"m0", "m1"} {
		if err := markAwaiting(other, "api", "5.5", "web", "40.4", id); err != nil {
			t.Fatal(err)
		}
	}
	got, want := Waiters(dir, "api", "5.5"), Waiters(other, "api", "5.5")
	if len(got) != 1 || len(want) != 1 {
		t.Fatalf("Owe wrote %+v, markAwaiting %+v", got, want)
	}
	for _, w := range []*Waiter{&got[0], &want[0]} {
		if w.Since < before || len(w.ReadAt) != len(w.Messages) {
			t.Fatalf("the wait does not carry its times: %+v", *w)
		}
		for _, at := range w.ReadAt {
			if at < before {
				t.Fatalf("a read time before the write: %+v", *w)
			}
		}
		w.Since, w.ReadAt = 0, nil
	}
	if !reflect.DeepEqual(got[0], want[0]) {
		t.Errorf("Owe's wait reads %+v, markAwaiting's %+v", got[0], want[0])
	}
	if _, err := os.Stat(filepath.Join(state.AwaitingPath(dir, "api"), "5.5", "web")); err != nil {
		t.Error(err)
	}
}
