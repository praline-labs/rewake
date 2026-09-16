package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

func holdThread(t *testing.T, home, thread string, at time.Time) {
	t.Helper()
	path := filepath.Join(home, "thread-writer-locks", thread+".lock")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

func TestReportsMarkOnlyKnownThreadChanges(t *testing.T) {
	for _, kind := range []string{"changed", "same", "mixed", "unknown", "other harness"} {
		t.Run(kind, func(t *testing.T) {
			dir := liveSession(t, "api")
			sender := otherRun(t, dir, "web")
			self, _ := registry.Lookup(dir, "api")
			self.Harness = "codex"
			self.HarnessPID = os.Getpid()
			self.HarnessStart = self.ServiceStart
			self.CodexHome = t.TempDir()
			if kind == "other harness" {
				self.Harness = "claude"
			}
			if err := registry.Update(dir, self); err != nil {
				t.Fatal(err)
			}
			if kind != "unknown" {
				holdThread(t, self.CodexHome, "now", time.Now())
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
			if code, _, errOut := run("turn-ended", turnPayload); code != 0 {
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
	if code != 0 || !strings.Contains(out, "new turn result") || !strings.Contains(out, "thread changed") || !strings.Contains(out, "resend the message") {
		t.Fatalf("warning missing: %d %s %s", code, out, errOut)
	}
}

func TestAQuestionPreservesTheThreadWarning(t *testing.T) {
	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			dir, web := questionSender(t)
			awaitStatus = func(_, _, id string, _ time.Duration) (inbox.Status, bool) {
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
			} else if !strings.Contains(out, "thread changed") {
				t.Fatalf("warning lost: %s", out)
			}
		})
	}
}
