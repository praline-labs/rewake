package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

func TestReportsMarkOnlyKnownThreadChanges(t *testing.T) {
	for _, kind := range []string{"changed", "same", "mixed", "unknown", "other harness"} {
		t.Run(kind, func(t *testing.T) {
			dir := liveSession(t, "api")
			sender := otherRun(t, dir, "web")
			self, _ := registry.Lookup(dir, "api")
			self.HarnessPID = os.Getpid()
			self.HarnessStart = self.ServiceStart
			if kind == "other harness" {
				self.Harness = "claude"
			}
			if err := registry.Update(dir, self); err != nil {
				t.Fatal(err)
			}

			t.Setenv(state.SessionEnv, "api")
			ids := []string{rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": sender.Epoch(), "toEpoch": self.Epoch(), "kind": "task", "text": "original task"})}
			if kind == "mixed" {
				ids = append(ids, rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": sender.Epoch(), "toEpoch": self.Epoch(), "kind": "task", "text": "new task"}))
			}
			for i, id := range ids {
				thread := "old"
				if kind == "same" || i == 1 {
					thread = "now"
				}
				path := filepath.Join(state.InboxPath(dir, "api"), "threads", id)
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(thread), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if code, _, errOut := run("inbox"); code != 0 {
				t.Fatal(errOut)
			}
			currentThread := "now"
			if kind == "unknown" || kind == "other harness" {
				currentThread = ""
			}
			if err := completeTurn(dir, self, inbox.TurnEnd{Text: "result"}, currentThread); err != nil {
				t.Fatal(err)
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
			want := kind == "changed" || kind == "mixed"
			if marked, _ := report["threadChanged"].(bool); marked != want {
				t.Errorf("kind=%s report=%s", kind, raw)
			}
			if !want {
				if _, exists := report["threadChanged"]; exists {
					t.Errorf("unexpected field: %s", raw)
				}
			}
			if len(report["inReplyTo"].([]any)) != len(ids) {
				t.Errorf("original waits lost: %s", raw)
			}
		})
	}
}

func TestInboxExplainsAReportFromAnotherThread(t *testing.T) {
	dir := liveSession(t, "api")
	t.Setenv(state.SessionEnv, "api")
	rawUnread(t, dir, "api", map[string]any{"toEpoch": epochOf(t, dir, "api"), "kind": "finished", "text": "new turn result", "threadChanged": true})
	code, out, errOut := run("inbox")
	if code != 0 || !strings.Contains(out, "new turn result") || !strings.Contains(out, "new turn result\nRewake: the reader's thread changed after delivery; this may not answer it, resend the message") {
		t.Fatalf("warning missing: %d %s %s", code, out, errOut)
	}
}

func TestAQuestionPreservesTheThreadWarning(t *testing.T) {
	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			dir, web := questionSender(t)
			awaitStatus = func(_, _, id string, _ time.Duration, _ func() bool) (inbox.Status, bool) {
				rawUnread(t, dir, "web", map[string]any{"from": "api", "kind": "finished", "toEpoch": web.Epoch(), "inReplyTo": []string{id}, "text": "new turn", "threadChanged": true})
				return inbox.Status{State: inbox.Delivered}, true
			}
			args := []string{"send", "api", "task", "--question", "--wait", "1"}
			if format == "json" {
				args = append(args, "--json")
			}
			code, out, errOut := run(args...)
			if code != 0 {
				t.Fatalf("question failed: %d %s", code, errOut)
			}
			if format == "json" {
				var model map[string]any
				if err := json.Unmarshal([]byte(out), &model); err != nil || model["threadChanged"] != true {
					t.Fatalf("warning lost: %s %v", out, err)
				}
			} else if !strings.Contains(out, "\nRewake: the reader's thread changed after delivery; this may not answer it, resend the message") {
				t.Fatalf("warning lost: %s", out)
			}
		})
	}
}
