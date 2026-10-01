package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

func reportObject(t *testing.T, dir, recipient string) map[string]any {
	t.Helper()
	files := finishedFor(t, dir, recipient)
	if len(files) != 1 {
		t.Fatalf("reports to %s: %v", recipient, files)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

// failedWaiters has api read a task from one and from two.
func failedWaiters(t *testing.T) string {
	t.Helper()
	dir := liveSession(t, "api")
	first := otherRun(t, dir, "one")
	second := otherRun(t, dir, "two")
	t.Setenv(state.SessionEnv, "api")
	for _, peer := range []registry.Session{first, second} {
		rawUnread(t, dir, "api", map[string]any{"from": peer.Name, "fromEpoch": peer.Epoch(), "toEpoch": epochOf(t, dir, "api"), "text": "task"})
	}
	run("inbox")
	return dir
}

func TestFailedTurnsReachEveryWaitingSender(t *testing.T) {
	check := func(t *testing.T, dir string) {
		t.Helper()
		for _, peer := range []string{"one", "two"} {
			report := reportObject(t, dir, peer)
			if report["kind"] != "error" || report["text"] != "verbatim failure" {
				t.Fatalf("report=%v", report)
			}
		}
	}
	t.Run("hook", func(t *testing.T) {
		dir := failedWaiters(t)
		if code, _, errOut := run("turn-ended", `{"hook_event_name":"StopFailure","error":"unknown","last_assistant_message":"verbatim failure"}`); code != 0 {
			t.Fatal(errOut)
		}
		check(t, dir)
	})
	t.Run("completion", func(t *testing.T) {
		dir := failedWaiters(t)
		self, _ := registry.Lookup(dir, "api")
		if err := completeTurn(dir, self, turnResult{Boundary: boundaryNow(t, dir, self), ID: "failed", Failed: true, Text: "verbatim failure"}, ""); err != nil {
			t.Fatal(err)
		}
		check(t, dir)
	})
}

// A Codex notify names its turn and carries no read boundary: its scope is
// unknown, so it reports nothing and its waits stay owed for the next end
// (docs/turn-end-recovery.md#the-operation). Codex's turn ends come through
// the gateway, which captures one.
func TestACodexNotifyReportsNothing(t *testing.T) {
	for _, payload := range []string{
		`{"type":"agent-turn-complete","turn-id":"failed","error":"verbatim failure","last-assistant-message":null}`,
		`{"type":"task_complete","turn_id":"failed","error":{"message":"verbatim failure"},"last_agent_message":null}`,
	} {
		t.Run(payload, func(t *testing.T) {
			dir := failedWaiters(t)
			if code, _, errOut := run("turn-ended", payload); code != 0 {
				t.Fatal(errOut)
			}
			for _, peer := range []string{"one", "two"} {
				if files := finishedFor(t, dir, peer); len(files) != 0 {
					t.Fatalf("reports to %s: %v", peer, files)
				}
			}
			if len(inbox.Waiters(dir, "api", epochOf(t, dir, "api"))) != 2 {
				t.Fatal("the refused end cleared its waits")
			}
		})
	}
}

func TestUnclaimedFailuresReachMainAndMainKeepsItsOwn(t *testing.T) {
	for _, self := range []bool{false, true} {
		t.Run(map[bool]string{true: "self", false: "leader"}[self], func(t *testing.T) {
			dir := liveSession(t, "api")
			t.Setenv(state.SessionEnv, "api")
			if self {
				markMain(t, dir, "api")
			} else {
				otherRun(t, dir, "leader")
				markMain(t, dir, "leader")
			}
			run("turn-ended", `{"hook_event_name":"StopFailure","error":"unknown","last_assistant_message":"failure"}`)
			if self {
				messages, err := inbox.PeekUnread(dir, "api", epochOf(t, dir, "api"))
				if err != nil || len(messages) != 1 || string(messages[0].Kind) != "error" {
					t.Fatalf("local error=%+v %v", messages, err)
				}
				if len(finishedFor(t, dir, "api")) != 0 {
					t.Fatal("main must not wake itself with its failure")
				}
			} else if reportObject(t, dir, "leader")["kind"] != "error" {
				t.Fatal("not an error")
			}
		})
	}
}

func TestEmptyCompletionAfterWorkReportsAnErrorWithoutText(t *testing.T) {
	dir := liveSession(t, "api")
	peer := otherRun(t, dir, "web")
	readFrom(t, dir, peer)
	run("turn-ended", `{"type":"agent-turn-complete","last-assistant-message":null}`)
	report := reportObject(t, dir, "web")
	if report["kind"] != "error" || report["text"] != "" {
		t.Fatalf("report=%v", report)
	}
}

func TestReadingAnErrorOwesNothing(t *testing.T) {
	dir := liveSession(t, "api")
	peer := otherRun(t, dir, "web")
	t.Setenv(state.SessionEnv, "api")
	rawUnread(t, dir, "api", map[string]any{"kind": "error", "text": "failure", "from": "web", "fromEpoch": peer.Epoch(), "toEpoch": epochOf(t, dir, "api")})
	run("inbox")
	if len(inbox.Waiters(dir, "api", epochOf(t, dir, "api"))) != 0 {
		t.Fatal("error created a reply obligation")
	}
	code, _, errOut := run("send", "web", "fake failure", "--error")
	if code != ExitUsage || !strings.Contains(errOut, "--error") {
		t.Fatalf("manual error accepted: %d %s", code, errOut)
	}
}

func TestIdentifiedFailuresAreNotRoutedAgainAfterTheirWaitsClear(t *testing.T) {
	dir := liveSession(t, "api")
	peer := otherRun(t, dir, "web")
	otherRun(t, dir, "leader")
	markMain(t, dir, "leader")
	readFrom(t, dir, peer)
	self, _ := registry.Lookup(dir, "api")
	event := turnResult{Boundary: boundaryNow(t, dir, self), ID: "failure-once", Failed: true, Text: "failure"}
	for range 2 {
		if err := completeTurn(dir, self, event, ""); err != nil {
			t.Fatal(err)
		}
	}
	if len(finishedFor(t, dir, "web")) != 1 || len(finishedFor(t, dir, "leader")) != 0 {
		t.Fatal("duplicate hook rerouted an already reported failure")
	}
}

func TestQuestionsReturnAnErrorOutcome(t *testing.T) {
	dir, peer := questionSender(t)
	awaitStatus = func(_, _, id string, _ time.Duration, _ func() bool) (inbox.Status, bool) {
		rawUnread(t, dir, "web", map[string]any{"kind": "error", "text": "failure", "toEpoch": peer.Epoch(), "inReplyTo": []string{id}})
		return inbox.Status{State: inbox.Delivered}, true
	}
	code, out, errOut := run("send", "api", "question", "--question", "--wait", "0.2", "--json")
	if code != ExitFailed || !strings.Contains(out, `"kind": "error"`) || !strings.Contains(out, `"answer": "failure"`) {
		t.Fatalf("question=%d %s %s", code, out, errOut)
	}
}
