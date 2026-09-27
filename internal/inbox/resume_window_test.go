package inbox

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// agedWaits moves back when a run's waits were recorded, and when each task
// in them was read, by age.
func agedWaits(t *testing.T, dir, run string, age time.Duration) {
	t.Helper()
	path, _ := awaitingPath(dir, "api", run)
	for _, waiter := range Waiters(dir, "api", run) {
		waiter.Since -= age.Nanoseconds()
		for index := range waiter.ReadAt {
			waiter.ReadAt[index] -= age.Nanoseconds()
		}
		if err := writeWaiter(filepath.Join(path, waiter.Name), waiter); err != nil {
			t.Fatal(err)
		}
	}
}

// awaitedBy is what the sender sees of each task it sent, given what became
// of the run it was written for.
func awaitedBy(dir string, what RecipientRun) map[string]AwaitedMessage {
	awaited := map[string]AwaitedMessage{}
	for _, item := range Awaited(dir, "web", "sender-epoch", func(string, string) RecipientRun { return what }) {
		awaited[item.ID] = item
	}
	return awaited
}

// A task read by a run that ended owing it is not lost while a resume of its
// conversation may still take it over: before any new run, and after a new
// one has started but has not swept the wait yet. It is lost once the wait is
// swept by a run in another conversation, when it was delivered into no
// conversation, or once the resume window has passed — and then main lets its
// grant go and a resume takes nothing over, so a task sent again is not done
// twice.
func TestATaskOfAnEndedRunIsLostOnlyWhenNoResumeCanTakeItOver(t *testing.T) {
	dir := stateDir(t)
	tasks := grantPending(t, dir, true, true)
	readIn(t, dir, "receiver-epoch", "thread-a", tasks[0])
	readIn(t, dir, "receiver-epoch", "", tasks[1])

	for _, what := range []RecipientRun{RunEnded, RunReplaced} {
		awaited := awaitedBy(dir, what)
		if item := awaited[tasks[0].ID]; !item.Resumable || item.Gone() {
			t.Errorf("run %v: a task a resume may take over reads as %+v", what, item)
		}
		if item := awaited[tasks[1].ID]; item.Resumable || !item.Gone() {
			t.Errorf("run %v: a task in no conversation reads as %+v", what, item)
		}
	}

	agedWaits(t, dir, "receiver-epoch", resumeWindow+time.Minute)
	if item := awaitedBy(dir, RunEnded)[tasks[0].ID]; item.Resumable || !item.Gone() {
		t.Errorf("past the window the task reads as %+v", item)
	}
	if open, _ := TaskOpen(dir, "api", tasks[0].ID); open {
		t.Error("past the window main still holds the task open")
	}
	if adopted := AdoptWaits(dir, "api", "resumed-epoch", "thread-a"); len(adopted) != 0 {
		t.Errorf("past the window a resume took over %v", adopted)
	}
}

// A wait gathers what is read from one sender until the next report, so it
// can have begun long before a task in it was read: that task is owed for the
// window from its own reading, not from the wait's beginning.
func TestATaskReadLateIntoALongWaitMayStillBeResumed(t *testing.T) {
	dir := stateDir(t)
	tasks := grantPending(t, dir, true, true)
	readIn(t, dir, "receiver-epoch", "thread-a", tasks[0])
	agedWaits(t, dir, "receiver-epoch", resumeWindow+time.Hour)
	readIn(t, dir, "receiver-epoch", "thread-a", tasks[1])
	if waits := Waiters(dir, "api", "receiver-epoch"); len(waits) != 1 || len(waits[0].Messages) != 2 {
		t.Fatalf("the two tasks are not in one wait: %+v", waits)
	}

	awaited := awaitedBy(dir, RunEnded)
	if item := awaited[tasks[0].ID]; item.Resumable || !item.Gone() {
		t.Errorf("the task read past the window reads as %+v", item)
	}
	if item := awaited[tasks[1].ID]; !item.Resumable || item.Gone() {
		t.Errorf("the task read late into the wait reads as %+v", item)
	}
	if open, _ := TaskOpen(dir, "api", tasks[1].ID); !open {
		t.Error("main lets go of the grant of a task a resume may still take over")
	}
	if adopted := AdoptWaits(dir, "api", "resumed-epoch", "thread-a"); !slices.Equal(adopted, []string{tasks[1].ID}) {
		t.Errorf("a resume took over %v", adopted)
	}
}

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
