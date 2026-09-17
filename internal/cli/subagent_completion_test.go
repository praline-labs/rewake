package cli

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

func TestSubagentCompletionDoesNotFinishTheParentTurn(t *testing.T) {
	for _, hook := range []string{"StopFailure", "Stop"} {
		t.Run(hook, func(t *testing.T) {
			dir := liveSession(t, "api")
			peer := otherRun(t, dir, "web")
			readFrom(t, dir, peer)
			child, err := json.Marshal(map[string]string{
				"hook_event_name": hook, "agent_id": "child-1", "agent_type": "Explore",
				"last_assistant_message": "child result; parent is still running",
			})
			if err != nil {
				t.Fatal(err)
			}
			run("turn-ended", string(child))
			if len(finishedFor(t, dir, "web")) != 0 || len(inbox.Waiters(dir, "api", epochOf(t, dir, "api"))) != 1 {
				t.Fatal("child completion consumed the parent turn's reply obligation")
			}
			// A root session can have an agent_type too; only agent_id identifies a child.
			run("turn-ended", `{"hook_event_name":"Stop","agent_type":"Explore","last_assistant_message":"parent completed the task"}`)
			files := finishedFor(t, dir, "web")
			if len(files) != 1 {
				t.Fatalf("parent reports = %d, want 1", len(files))
			}
			raw, err := os.ReadFile(files[0])
			if err != nil {
				t.Fatal(err)
			}
			var report inbox.Message
			if err := json.Unmarshal(raw, &report); err != nil {
				t.Fatal(err)
			}
			if report.Kind != inbox.Finished || report.Text != "parent completed the task" {
				t.Fatalf("wrong parent result: %+v", report)
			}
		})
	}
}
