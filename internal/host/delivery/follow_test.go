package delivery

import (
	"context"
	"testing"

	"github.com/praline-labs/rewake/internal/core/mail"
)

// A new run forgets the waits of the earlier runs of its name: it owes none
// of it, and the sender reads the task as lost.
func TestANewRunForgetsTheEarlierWaits(t *testing.T) {
	dir := stateDir(t)
	task := pendingTasks(t, dir, 1)[0]
	readIn(t, dir, "receiver-epoch", "thread-a", task)
	s := batchServer(dir)
	s.Epoch = "next-epoch"
	s.followEarlierRun(context.Background())
	if len(mail.Open(dir, "api", "receiver-epoch").Waiters()) != 0 || len(mail.Open(dir, "api", "next-epoch").Waiters()) != 0 {
		t.Fatalf("waits stayed or were taken over")
	}
	if item := awaitedBy(dir)[task.ID]; !item.Gone() {
		t.Errorf("the task reads as %+v", item)
	}
}
