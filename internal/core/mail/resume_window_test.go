package mail

import (
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
	for _, item := range must(Awaited(dir, "web", "sender-epoch", func(string, string) RecipientRun { return what })) {
		awaited[item.ID] = item
	}
	return awaited
}

// A task read by a run that ended owing it is lost for its sender, in a
// conversation or not: no release reopens a worker's conversation. The old
// adoption still takes a wait over inside the resume window, and nothing past it.
func TestATaskOfAnEndedRunIsLostAndAdoptionKeepsItsWindow(t *testing.T) {
	dir := stateDir(t)
	tasks := pendingTasks(t, dir, 2)
	readIn(t, dir, "receiver-epoch", "thread-a", tasks[0])
	readIn(t, dir, "receiver-epoch", "", tasks[1])

	for _, what := range []RecipientRun{RunEnded, RunReplaced} {
		awaited := awaitedBy(dir, what)
		if item := awaited[tasks[0].ID]; !item.Gone() {
			t.Errorf("run %v: a task in a conversation reads as %+v", what, item)
		}
		if item := awaited[tasks[1].ID]; !item.Gone() {
			t.Errorf("run %v: a task in no conversation reads as %+v", what, item)
		}
	}

	agedWaits(t, dir, "receiver-epoch", resumeWindow+time.Minute)
	if item := awaitedBy(dir, RunEnded)[tasks[0].ID]; !item.Gone() {
		t.Errorf("past the window the task reads as %+v", item)
	}
	if adopted := must(AdoptWaits(dir, "api", "resumed-epoch", "thread-a")); len(adopted) != 0 {
		t.Errorf("past the window a resume took over %v", adopted)
	}
}

// A wait gathers what is read from one sender until the next report, so it
// can have begun long before a task in it was read: adoption counts the window
// from the task's own reading, not from the wait's beginning.
func TestATaskReadLateIntoALongWaitMayStillBeAdopted(t *testing.T) {
	dir := stateDir(t)
	tasks := pendingTasks(t, dir, 2)
	readIn(t, dir, "receiver-epoch", "thread-a", tasks[0])
	agedWaits(t, dir, "receiver-epoch", resumeWindow+time.Hour)
	readIn(t, dir, "receiver-epoch", "thread-a", tasks[1])
	if waits := Waiters(dir, "api", "receiver-epoch"); len(waits) != 1 || len(waits[0].Messages) != 2 {
		t.Fatalf("the two tasks are not in one wait: %+v", waits)
	}

	awaited := awaitedBy(dir, RunEnded)
	if item := awaited[tasks[0].ID]; !item.Gone() {
		t.Errorf("the task read past the window reads as %+v", item)
	}
	if adopted := must(AdoptWaits(dir, "api", "resumed-epoch", "thread-a")); !slices.Equal(adopted, []string{tasks[1].ID}) {
		t.Errorf("a resume took over %v", adopted)
	}
}
