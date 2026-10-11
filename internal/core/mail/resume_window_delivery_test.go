package mail

import (
	"context"
	"testing"
)

// A new run in another conversation sweeps the wait, and then the task is
// lost for good.
func TestATaskSweptByARunInAnotherConversationIsLost(t *testing.T) {
	dir := stateDir(t)
	task := grantPending(t, dir, true)[0]
	readIn(t, dir, "receiver-epoch", "thread-a", task)
	s := batchServer(dir)
	s.Epoch = "next-epoch"
	s.Thread = func() (string, error) { return "thread-b", nil }
	s.followEarlierRun(context.Background())
	if item := awaitedBy(dir, RunReplaced)[task.ID]; item.Resumable || !item.Gone() {
		t.Errorf("swept by a run in another conversation, the task reads as %+v", item)
	}
}
