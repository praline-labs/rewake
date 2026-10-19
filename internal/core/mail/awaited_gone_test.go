package mail

import "testing"

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
// conversation or not: no release reopens a worker's conversation.
func TestATaskOfAnEndedRunIsLost(t *testing.T) {
	dir := stateDir(t)
	tasks := pendingTasks(t, dir, 2)
	readIn(t, dir, "receiver-epoch", "thread-a", tasks[0])
	readIn(t, dir, "receiver-epoch", "", tasks[1])
	for _, what := range []RecipientRun{RunEnded, RunReplaced} {
		awaited := awaitedBy(dir, what)
		for _, task := range tasks {
			if item := awaited[task.ID]; !item.Gone() {
				t.Errorf("run %v: the task reads as %+v", what, item)
			}
		}
	}
}
