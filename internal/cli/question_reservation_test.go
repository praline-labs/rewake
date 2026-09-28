package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

func questionSender(t *testing.T) (string, registry.Session) {
	t.Helper()
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	t.Setenv(state.SessionEnv, "web")
	t.Setenv(epochEnv, web.Epoch())
	old := awaitStatus
	t.Cleanup(func() { awaitStatus = old })
	return dir, web
}

func TestAnAnswerSurvivesFailedOutput(t *testing.T) {
	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			dir, web := questionSender(t)
			awaitStatus = func(_, _, id string, _ time.Duration, _ func() bool) (inbox.Status, bool) {
				rawUnread(t, dir, "web", map[string]any{"from": "api", "kind": "finished", "toEpoch": web.Epoch(), "inReplyTo": []string{id}, "text": "irreplaceable result"})
				return inbox.Status{State: inbox.Delivered}, true
			}
			args := []string{"send", "api", "question", "--question", "--wait", "0.2"}
			if format == "json" {
				args = append(args, "--json")
			}
			var stderr bytes.Buffer
			code := Run(args, brokenWriter{}, &stderr)
			if code != ExitFailed {
				t.Errorf("exit=%d, want failed output; stderr=%s", code, stderr.String())
			}
			code, out, errOut := run("inbox")
			if code != 0 || !strings.Contains(out, "irreplaceable result") {
				t.Errorf("answer lost: %d %s %s", code, out, errOut)
			}
		})
	}
}

func TestInboxLeavesAReservedAnswerForSend(t *testing.T) {
	dir, web := questionSender(t)
	awaitStatus = func(_, _, id string, _ time.Duration, _ func() bool) (inbox.Status, bool) {
		// Existing waits also reserve their answer, independently of when send
		// first creates the mark.
		marks := state.AnsweringPath(dir, "web")
		if err := os.MkdirAll(marks, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(marks, id), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		rawUnread(t, dir, "web", map[string]any{"from": "api", "kind": "finished", "toEpoch": web.Epoch(), "inReplyTo": []string{id}, "text": "reserved answer"})
		rawUnread(t, dir, "web", map[string]any{"from": "api", "kind": "notify", "toEpoch": web.Epoch(), "text": "unrelated update"})
		code, out, errOut := run("inbox")
		if code != 0 || !strings.Contains(out, "unrelated update") || strings.Contains(out, "reserved answer") {
			t.Errorf("inbox stole reserved answer: %d %s %s", code, out, errOut)
		}
		return inbox.Status{State: inbox.Delivered}, true
	}
	code, out, errOut := run("send", "api", "question", "--question", "--wait", "0.2")
	if code != 0 || !strings.Contains(out, "reserved answer") {
		t.Errorf("answer not returned: %d %s %s", code, out, errOut)
	}
}

func TestEveryQuestionReceivesASharedAnswer(t *testing.T) {
	dir, web := questionSender(t)
	ids := make(chan string, 2)
	ready := make(chan struct{})
	awaitStatus = func(_, _, id string, _ time.Duration, _ func() bool) (inbox.Status, bool) {
		ids <- id
		<-ready
		return inbox.Status{State: inbox.Delivered}, true
	}
	results := make(chan string, 2)
	for range 2 {
		go func() {
			code, out, errOut := run("send", "api", "question", "--question", "--wait", "0.5")
			if code != 0 || !strings.Contains(out, "shared result") {
				results <- out + errOut
				return
			}
			results <- ""
		}()
	}
	first, second := <-ids, <-ids
	rawUnread(t, dir, "web", map[string]any{"from": "api", "kind": "finished", "toEpoch": web.Epoch(), "inReplyTo": []string{first, second}, "text": "shared result"})
	close(ready)
	for range 2 {
		if failure := <-results; failure != "" {
			t.Errorf("question lost its answer: %s", failure)
		}
	}
	if code, out, _ := run("inbox"); code != 0 || strings.Contains(out, "shared result") {
		t.Errorf("consumed report still unread: %s", out)
	}
}

func TestAQuestionToASilentSessionIsRefused(t *testing.T) {
	dir, _ := questionSender(t)
	target, err := registry.Lookup(dir, "api")
	if err != nil {
		t.Fatal(err)
	}
	target.Role = "main"
	if err := registry.Update(dir, target); err != nil {
		t.Fatal(err)
	}
	awaitStatus = func(_, _, _ string, _ time.Duration, _ func() bool) (inbox.Status, bool) {
		return inbox.Status{State: inbox.Delivered}, true
	}
	code, _, errOut := run("send", "api", "question", "--question", "--wait", "0")
	if code != ExitUsage || !strings.Contains(errOut, "--notify") || !strings.Contains(errOut, "report") {
		t.Errorf("silent target accepted: %d %s", code, errOut)
	}
	files, _ := filepath.Glob(filepath.Join(state.InboxPath(dir, "api"), "*.json"))
	if len(files) > 0 {
		t.Errorf("refused question was published: %v", files)
	}
}
