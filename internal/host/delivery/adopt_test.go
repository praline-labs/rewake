package delivery

import (
	"context"
	"slices"
	"testing"

	"github.com/praline-labs/rewake/internal/core/mail"
)

func owedBy(dir, run string) []string {
	var ids []string
	for _, waiter := range mail.Waiters(dir, "api", run) {
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
	if open, found := mail.TaskOpen(dir, "api", tasks[1].ID); !open || !found {
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
	if open, found := mail.TaskOpen(dir, "api", tasks[0].ID); !open || !found {
		t.Error("the task taken over is closed")
	}
	if open, found := mail.TaskOpen(dir, "api", tasks[1].ID); open || !found {
		t.Error("the task of another conversation is still open")
	}
	if mail.Settled(dir, "api", tasks[0].ID) {
		t.Error("the task taken over is settled")
	}

	// The sender sees the task taken over as waited on, and the other as
	// gone with the run it was written for.
	runOf := func(_, run string) mail.RecipientRun {
		if run == "resumed-epoch" {
			return mail.RunLive
		}
		return mail.RunReplaced
	}
	awaited := map[string]mail.AwaitedMessage{}
	for _, item := range must(mail.Awaited(dir, "web", "sender-epoch", runOf)) {
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
	waiter := mail.Waiter{Name: "web", Epoch: "sender-epoch", Messages: []string{tasks[0].ID}}
	report := mail.Message{ID: mail.ReportID("api", "resumed-epoch", waiter), From: "api", FromEpoch: "resumed-epoch", To: "web", ToEpoch: "sender-epoch", Kind: mail.Finished, InReplyTo: waiter.Messages}
	if err := mail.Put(dir, report); err != nil {
		t.Fatal(err)
	}
	if err := mail.ClearAwaiting(dir, "api", "resumed-epoch", waiter); err != nil {
		t.Fatal(err)
	}
	for _, item := range must(mail.Awaited(dir, "web", "sender-epoch", runOf)) {
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
	if open, _ := mail.TaskOpen(dir, "api", task.ID); open {
		t.Error("the task is still open")
	}
}
