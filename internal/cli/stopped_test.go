package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

func TestStoppedKeepsWorkForTheHumanContinuation(t *testing.T) {
	dir := liveSession(t, "api")
	peer := otherRun(t, dir, "web")
	task := readFrom(t, dir, peer)
	self, err := registry.Lookup(dir, "api")
	if err != nil {
		t.Fatal(err)
	}
	event := turnResult{ID: "thread/stopped", Stopped: true, Text: "the person at the keyboard stopped this turn"}
	if err := completeTurn(dir, self, event, "thread"); err != nil {
		t.Fatal(err)
	}
	if err := completeTurn(dir, self, event, "thread"); err != nil {
		t.Fatal(err)
	}
	report := reportObject(t, dir, "web")
	if report["kind"] != "stopped" || len(inbox.Waiters(dir, "api", self.Epoch())) != 1 {
		t.Fatal("stopped settled the task or lost its kind")
	}
	if err := completeTurn(dir, self, turnResult{ID: "thread/continued", Text: "continued result"}, "thread"); err != nil {
		t.Fatal(err)
	}
	files := finishedFor(t, dir, "web")
	if len(files) != 2 || len(inbox.Waiters(dir, "api", self.Epoch())) != 0 {
		t.Fatal("continuation did not settle the original task")
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var message inbox.Message
		if err := json.Unmarshal(raw, &message); err != nil {
			t.Fatal(err)
		}
		if len(message.InReplyTo) != 1 || message.InReplyTo[0] != task || inbox.Owed(message) {
			t.Fatalf("wrong continuation contract: %+v", message)
		}
	}
}

func TestStoppedFallsBackToMainWithoutCreatingReplyWork(t *testing.T) {
	dir := liveSession(t, "api")
	otherRun(t, dir, "leader")
	markMain(t, dir, "leader")
	self, err := registry.Lookup(dir, "api")
	if err != nil {
		t.Fatal(err)
	}
	event := turnResult{ID: "thread/stop", Stopped: true, Text: "the person at the keyboard stopped this turn"}
	for range 2 {
		if err := completeTurn(dir, self, event, "thread"); err != nil {
			t.Fatal(err)
		}
	}
	if reportObject(t, dir, "leader")["kind"] != "stopped" {
		t.Fatal("main did not receive stopped")
	}
	if _, err := parse([]string{"send", "api", "manual", "--stopped"}); err == nil {
		t.Fatal("manual stopped reports must be refused")
	}
}

func TestAStoppedQuestionReturnsAndLeavesTheLaterResultReadable(t *testing.T) {
	dir, web := questionSender(t)
	var question string
	awaitStatus = func(_, _, id string, _ time.Duration) (inbox.Status, bool) {
		question = id
		rawUnread(t, dir, "web", map[string]any{"from": "api", "kind": "stopped", "toEpoch": web.Epoch(), "inReplyTo": []string{id}, "text": "the person at the keyboard stopped this turn"})
		return inbox.Status{State: inbox.Delivered}, true
	}
	code, _, errOut := run("send", "api", "work", "--question", "--wait", "1")
	if code != ExitFailed {
		t.Fatalf("stopped question: %d %s", code, errOut)
	}
	rawUnread(t, dir, "web", map[string]any{"from": "api", "kind": "finished", "toEpoch": web.Epoch(), "inReplyTo": []string{question}, "text": "continued result"})
	t.Setenv(state.SessionEnv, "web")
	t.Setenv(state.EpochEnv, web.Epoch())
	code, out, errOut := run("inbox")
	if code != 0 || !strings.Contains(out, "continued result") {
		t.Fatalf("continuation disappeared: %d %s %s", code, out, errOut)
	}
}
