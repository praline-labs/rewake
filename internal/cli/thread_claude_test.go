package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// A Claude Code turn end names its conversation in the hook's session_id, and
// that is what the delivery's conversation is compared with. Either side
// unknown leaves the field out rather than guessing a change.
func TestAClaudeTurnEndComparesItsSessionID(t *testing.T) {
	for _, tc := range []struct {
		name      string
		delivered string
		payload   map[string]any
		want      bool
	}{
		{"cleared", "conv-a", map[string]any{"hook_event_name": "Stop", "session_id": "conv-b", "last_assistant_message": "done"}, true},
		{"same", "conv-a", map[string]any{"hook_event_name": "Stop", "session_id": "conv-a", "last_assistant_message": "done"}, false},
		{"failed in another", "conv-a", map[string]any{"hook_event_name": "StopFailure", "session_id": "conv-b", "last_assistant_message": "broke", "error": "api_error"}, true},
		{"unnamed", "conv-a", map[string]any{"hook_event_name": "Stop", "last_assistant_message": "done"}, false},
		{"unpinned", "", map[string]any{"hook_event_name": "Stop", "session_id": "conv-b", "last_assistant_message": "done"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := liveSession(t, "api")
			sender := otherRun(t, dir, "web")
			self, _ := registry.Lookup(dir, "api")
			t.Setenv(state.SessionEnv, "api")
			id := rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": sender.Epoch(), "toEpoch": self.Epoch(), "kind": "task", "text": "the task"})
			if tc.delivered != "" {
				path := filepath.Join(state.InboxPath(dir, "api"), "threads", id)
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(tc.delivered), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if code, _, errOut := run("inbox"); code != 0 {
				t.Fatal(errOut)
			}
			payload, _ := json.Marshal(tc.payload)
			if code, _, errOut := run("turn-ended", string(payload)); code != ExitOK {
				t.Fatal(errOut)
			}
			files := finishedFor(t, dir, "web")
			if len(files) != 1 {
				t.Fatalf("reports=%v", files)
			}
			raw, err := os.ReadFile(files[0])
			if err != nil {
				t.Fatal(err)
			}
			var report map[string]any
			if err := json.Unmarshal(raw, &report); err != nil {
				t.Fatal(err)
			}
			marked, present := report["threadChanged"]
			if tc.want && marked != true || !tc.want && present {
				t.Errorf("report = %s, want threadChanged %v", raw, tc.want)
			}
		})
	}
}

// A hook's payload names the conversation: its session_id, on either end.
func TestAHookPayloadNamesTheConversation(t *testing.T) {
	for _, tc := range []struct {
		payload string
		want    string
	}{
		{`{"hook_event_name":"Stop","session_id":"conv-a","last_assistant_message":"x"}`, "conv-a"},
		{`{"hook_event_name":"StopFailure","session_id":"conv-a","last_assistant_message":"x"}`, "conv-a"},
	} {
		event, ok := completedTurn([]byte(tc.payload))
		if !ok || event.Thread != tc.want {
			t.Errorf("%s: thread %q (ok %v), want %q", tc.payload, event.Thread, ok, tc.want)
		}
	}
}
