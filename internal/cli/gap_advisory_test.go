package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/boottime"
	"github.com/iiiokojiadbi/rewake/internal/harness/codex/gateway"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// A run seen only as an active status and then idle, its turn unknown, may
// have been work or a compaction; a goal's turn that fails before its first
// item shows no proof of work. Each reaches the waiter as an advisory report
// that settles nothing, and the next turn that finishes settles the task.
func TestARunWithoutProofOfWorkIsAdvisory(t *testing.T) {
	active := map[string]any{"threadId": "A", "status": map[string]string{"type": "active"}}
	idle := map[string]any{"threadId": "A", "status": map[string]string{"type": "idle"}}
	runs := map[string]struct {
		events [][2]any
		text   string
	}{
		"a compaction's gap":                   {[][2]any{{"thread/status/changed", active}, {"item/completed", map[string]any{"threadId": "A", "turnId": "G", "item": map[string]string{"id": "c", "type": "contextCompaction"}}}, {"thread/status/changed", idle}}, "passed unseen"},
		"a work gap":                           {[][2]any{{"thread/status/changed", active}, {"item/completed", map[string]any{"threadId": "A", "turnId": "G", "item": map[string]string{"id": "u", "type": "userMessage"}}}, {"item/completed", map[string]any{"threadId": "A", "turnId": "G", "item": map[string]string{"id": "a", "type": "agentMessage", "text": "done"}}}, {"thread/status/changed", idle}}, "passed unseen"},
		"a goal's turn failed before its item": {[][2]any{{"thread/status/changed", active}, {"turn/started", map[string]any{"threadId": "A", "turn": map[string]any{"id": "G", "items": []any{}, "status": "inProgress"}}}, {"turn/completed", map[string]any{"threadId": "A", "turn": map[string]any{"id": "G", "items": []any{}, "status": "failed", "error": map[string]string{"message": "usage limit reached"}}}}, {"thread/status/changed", idle}}, "ended without rewake seeing what it did; usage limit reached"},
	}
	for name, run := range runs {
		t.Run(name, func(t *testing.T) {
			dir := liveSession(t, "api")
			peer := otherRun(t, dir, "web")
			self, _ := registry.Lookup(dir, "api")
			completed := make(chan gateway.Completion, 4)
			_, ui, native := gatewayWireFixture(t, self.Epoch(), func(result gateway.Completion) { completed <- result })
			wireExchange(t, ui, native, 1, "thread/start", map[string]any{"threadSource": "user", "runtimeWorkspaceRoots": []string{}}, map[string]any{"thread": map[string]any{"id": "A", "canAcceptDirectInput": true}})
			readFrom(t, dir, peer)
			for _, event := range run.events {
				reviewNativeEvent(t, ui, native, event[0].(string), event[1])
			}
			gap := reviewOutcome(t, completed)
			if gap.Kind != string(inbox.Stopped) || !strings.Contains(gap.Text, run.text) {
				t.Fatalf("gap outcome %+v", gap)
			}
			reviewPublish(t, dir, self, gap)
			if len(finishedFor(t, dir, peer.Name)) != 1 {
				t.Fatal("the gap's advisory report did not reach the waiter")
			}
			if len(inbox.Waiters(dir, self.Name, self.Epoch())) != 1 {
				t.Fatal("a gap settled the wait")
			}
			wireExchange(t, ui, native, 2, "turn/start", map[string]any{"threadId": "A", "input": []any{}}, map[string]any{"turn": map[string]any{"id": "W", "items": []any{}, "status": "inProgress"}})
			reviewNativeEvent(t, ui, native, "turn/started", map[string]any{"threadId": "A", "turn": map[string]any{"id": "W", "items": []any{}, "status": "inProgress"}})
			reviewNativeEvent(t, ui, native, "item/completed", map[string]any{"threadId": "A", "turnId": "W", "item": map[string]string{"id": "w", "type": "agentMessage", "text": "the report"}})
			reviewNativeEvent(t, ui, native, "turn/completed", map[string]any{"threadId": "A", "turn": map[string]any{"id": "W", "items": []any{}, "status": "completed"}})
			finished := reviewOutcome(t, completed)
			if finished.Kind != string(inbox.Finished) || finished.Text != "the report" {
				t.Fatalf("finished outcome %+v", finished)
			}
			reviewPublish(t, dir, self, finished)
			if len(inbox.Waiters(dir, self.Name, self.Epoch())) != 0 {
				t.Fatal("the turn that finished did not settle the wait")
			}
		})
	}
}

// A turn whose reply comes after its advisory report still reports its own
// outcome, a stopped one included, with its times: the advisory's identity is
// its own in the gateway and in the turn receipts, and the pending mark made
// during the turn is taken by the turn's own outcome, not by the advisory.
func TestAProvenStopAfterTheAdvisoryReportsWithItsTimes(t *testing.T) {
	dir := liveSession(t, "api")
	peer := otherRun(t, dir, "web")
	self, _ := registry.Lookup(dir, "api")
	completed := make(chan gateway.Completion, 4)
	_, ui, native := gatewayWireFixture(t, self.Epoch(), func(result gateway.Completion) { completed <- result })
	wireExchange(t, ui, native, 1, "thread/start", map[string]any{"threadSource": "user", "runtimeWorkspaceRoots": []string{}}, map[string]any{"thread": map[string]any{"id": "A", "canAcceptDirectInput": true}})
	readFrom(t, dir, peer)
	if err := ui.write(map[string]any{"id": 2, "method": "turn/start", "params": map[string]any{"threadId": "A", "input": []any{}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := native.read(); err != nil {
		t.Fatal(err)
	}
	reviewNativeEvent(t, ui, native, "turn/started", map[string]any{"threadId": "A", "turn": map[string]any{"id": "U", "items": []any{}, "status": "inProgress"}})
	if err := inbox.MarkPending(dir, self.Name, self.Epoch(), "still at it", boottime.Now()); err != nil {
		t.Fatal(err)
	}
	reviewNativeEvent(t, ui, native, "turn/completed", map[string]any{"threadId": "A", "turn": map[string]any{"id": "U", "items": []any{}, "status": "interrupted"}})
	advisory := reviewOutcome(t, completed)
	if advisory.Kind != string(inbox.Stopped) || !strings.HasPrefix(advisory.Text, "a turn of this conversation ended without rewake") {
		t.Fatalf("advisory outcome %+v", advisory)
	}
	reviewPublish(t, dir, self, advisory)
	if _, err := os.Stat(filepath.Join(state.InboxPath(dir, self.Name), "pending", "mark.json")); err != nil {
		t.Fatalf("the advisory took the pending mark: %v", err)
	}
	if err := native.write(map[string]any{"id": 2, "result": map[string]any{"turn": map[string]any{"id": "U", "items": []any{}, "status": "inProgress"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := ui.read(); err != nil {
		t.Fatal(err)
	}
	stopped := reviewOutcome(t, completed)
	if stopped.Kind != string(inbox.Stopped) || stopped.Text != "the person at the keyboard stopped this turn" || stopped.Ended == 0 {
		t.Fatalf("the turn's own outcome %+v", stopped)
	}
	reviewPublish(t, dir, self, stopped)
	if got := reportsTo(t, dir, peer.Name); len(got) != 2 || got[0].Text == got[1].Text {
		t.Fatalf("web holds %+v; want the advisory and the turn's own stop", got)
	}
	if _, err := os.Stat(filepath.Join(state.InboxPath(dir, self.Name), "pending", "mark.json")); !os.IsNotExist(err) {
		t.Fatalf("the pending mark was not taken by the turn's own outcome: %v", err)
	}
}
