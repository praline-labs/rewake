package mail

import (
	"fmt"
	"testing"
	"time"
)

// sentIn writes a task from one run of web, created at the given age, and has
// api's run read it.
func sentIn(t *testing.T, dir, run string, age time.Duration) Message {
	t.Helper()
	m := message("job of " + run)
	m.Kind, m.FromEpoch, m.ToEpoch = Task, run, "receiver-epoch"
	m.CreatedAt = time.Now().Add(-age)
	m.ID = fmt.Sprintf("%019d-%s", m.CreatedAt.UnixNano(), run)
	if err := Put(dir, m); err != nil {
		t.Fatal(err)
	}
	readIn(t, dir, "receiver-epoch", "", m)
	return m
}

func awaitedAs(dir, run string, what RecipientRun) map[string]AwaitedMessage {
	awaited := map[string]AwaitedMessage{}
	for _, item := range must(Awaited(dir, "web", run, func(string, string) RecipientRun { return what })) {
		awaited[item.ID] = item
	}
	return awaited
}

// A restarted session sees what the run it replaced was owed, as lost when the
// recipient's run has ended, and nothing of the runs before that one.
func TestARestartedRunSeesThePreviousRunsTasks(t *testing.T) {
	dir := stateDir(t)
	oldest := sentIn(t, dir, "run-1", 3*time.Hour)
	previous := sentIn(t, dir, "run-2", time.Hour)
	awaited := awaitedAs(dir, "run-3", RunEnded)
	if item, ok := awaited[previous.ID]; !ok || !item.Gone() {
		t.Errorf("the previous run's task reads as %+v", item)
	}
	if _, ok := awaited[oldest.ID]; ok || len(awaited) != 1 {
		t.Errorf("listed %d tasks, want only the previous run's: %+v", len(awaited), awaited)
	}
	if own := awaitedAs(dir, "run-2", RunEnded); len(own) != 1 {
		t.Errorf("the run itself sees %d tasks, want its own only", len(own))
	}
}

// What the previous run was already answered is not listed.
func TestARestartedRunDoesNotSeeAnAnsweredTask(t *testing.T) {
	dir := stateDir(t)
	task := sentIn(t, dir, "run-1", time.Hour)
	report := Message{ID: NewID(), From: "api", To: "web", ToEpoch: "run-1", Kind: Finished, InReplyTo: []string{task.ID}, CreatedAt: time.Now()}
	if err := Put(dir, report); err != nil {
		t.Fatal(err)
	}
	if awaited := awaitedAs(dir, "run-2", RunEnded); len(awaited) != 0 {
		t.Errorf("listed %+v", awaited)
	}
}
