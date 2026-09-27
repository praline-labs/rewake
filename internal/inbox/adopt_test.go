package inbox

import (
	"context"
	"slices"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

// readIn has run read a task delivered into a conversation.
func readIn(t *testing.T, dir, run, thread string, m Message) {
	t.Helper()
	if err := state.WithMailboxLock(context.Background(), dir, "api", func() error {
		if err := recordDeliveryThread(dir, "api", m.ID, thread); err != nil {
			return err
		}
		if err := linkUnread(dir, "api", m.ID); err != nil {
			return err
		}
		return MarkRead(dir, "api", run, m, true)
	}); err != nil {
		t.Fatal(err)
	}
}

func owedBy(dir, run string) []string {
	var ids []string
	for _, waiter := range Waiters(dir, "api", run) {
		ids = append(ids, waiter.Messages...)
	}
	return ids
}

// A run that resumed a conversation takes over what the run before it owed
// there, once it knows the conversation, and not before; a task that went
// into another conversation it does not take, and that one is closed.
func TestAResumedRunTakesOverWhatItsConversationOwes(t *testing.T) {
	dir := stateDir(t)
	tasks := grantPending(t, dir, true, true)
	readIn(t, dir, "receiver-epoch", "thread-a", tasks[0])
	readIn(t, dir, "receiver-epoch", "thread-b", tasks[1])

	thread := ""
	s := batchServer(dir)
	s.Epoch = "resumed-epoch"
	s.Thread = func() (string, error) { return thread, nil }
	s.followEarlierRun(context.Background())
	if owed := owedBy(dir, "receiver-epoch"); len(owed) != 2 {
		t.Fatalf("the earlier run's waits went before the conversation was known: %v", owed)
	}
	if open, found := TaskOpen(dir, "api", tasks[1].ID); !open || !found {
		t.Fatal("a task the earlier run owes is closed while the new run does not know its conversation")
	}

	thread = "thread-a"
	s.followEarlierRun(context.Background())
	if owed := owedBy(dir, "resumed-epoch"); !slices.Equal(owed, []string{tasks[0].ID}) {
		t.Fatalf("the resumed run owes %v", owed)
	}
	if owed := owedBy(dir, "receiver-epoch"); len(owed) != 0 {
		t.Fatalf("the earlier run's waits stayed: %v", owed)
	}
	if open, found := TaskOpen(dir, "api", tasks[0].ID); !open || !found {
		t.Error("the task taken over is closed")
	}
	if open, found := TaskOpen(dir, "api", tasks[1].ID); open || !found {
		t.Error("the task of another conversation is still open")
	}
	if Settled(dir, "api", tasks[0].ID) {
		t.Error("the task taken over is settled")
	}

	// The sender sees the task taken over as waited on, and the other as
	// gone with the run it was written for.
	runOf := func(_, run string) RecipientRun {
		if run == "resumed-epoch" {
			return RunLive
		}
		return RunReplaced
	}
	awaited := map[string]AwaitedMessage{}
	for _, item := range Awaited(dir, "web", "sender-epoch", runOf) {
		awaited[item.ID] = item
	}
	if item, ok := awaited[tasks[0].ID]; !ok || item.Gone() {
		t.Errorf("the task taken over is not waited on: %+v", item)
	}
	if item, ok := awaited[tasks[1].ID]; !ok || !item.Gone() {
		t.Errorf("the task of another conversation is not gone: %+v", item)
	}

	// The resumed run's report settles the task, though that run is not the
	// one the task was written for.
	waiter := Waiter{Name: "web", Epoch: "sender-epoch", Messages: []string{tasks[0].ID}}
	report := Message{ID: ReportID("api", "resumed-epoch", waiter), From: "api", FromEpoch: "resumed-epoch", To: "web", ToEpoch: "sender-epoch", Kind: Finished, InReplyTo: waiter.Messages}
	if err := Put(dir, report); err != nil {
		t.Fatal(err)
	}
	ClearAwaiting(dir, "api", "resumed-epoch", waiter)
	for _, item := range Awaited(dir, "web", "sender-epoch", runOf) {
		if item.ID == tasks[0].ID {
			t.Errorf("reported on by the resumed run and still awaited: %+v", item)
		}
	}
}

// A session whose harness names no conversation continues none, and forgets
// the earlier run's waits at once.
func TestARunWithoutAConversationForgetsTheEarlierWaits(t *testing.T) {
	dir := stateDir(t)
	task := grantPending(t, dir, true)[0]
	readIn(t, dir, "receiver-epoch", "thread-a", task)
	s := batchServer(dir)
	s.Epoch = "next-epoch"
	s.followEarlierRun(context.Background())
	if owed := owedBy(dir, "receiver-epoch"); len(owed) != 0 {
		t.Fatalf("the earlier run's waits stayed: %v", owed)
	}
	if owed := owedBy(dir, "next-epoch"); len(owed) != 0 {
		t.Fatalf("taken over without a conversation: %v", owed)
	}
	if open, _ := TaskOpen(dir, "api", task.ID); open {
		t.Error("the task is still open")
	}
}

// Whether a task is open, as main's wrapper asks it of a grant it holds.
func TestTaskOpenFollowsTheTask(t *testing.T) {
	dir := stateDir(t)
	tasks := grantPending(t, dir, true, true, true)
	if open, found := TaskOpen(dir, "api", tasks[0].ID); !open || !found {
		t.Error("a task on its way is not open")
	}
	if err := writeStatus(dir, "api", tasks[1].ID, Result{State: Failed}); err != nil {
		t.Fatal(err)
	}
	if open, found := TaskOpen(dir, "api", tasks[1].ID); open || !found {
		t.Error("a failed task is open")
	}
	if err := writeStatus(dir, "api", tasks[2].ID, Result{State: Delivered, Withdrawn: true}); err != nil {
		t.Fatal(err)
	}
	if open, found := TaskOpen(dir, "api", tasks[2].ID); open || !found {
		t.Error("a task taken back is open")
	}
	if _, found := TaskOpen(dir, "api", NewID()); found {
		t.Error("a message never written was found")
	}
}
