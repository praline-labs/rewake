package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/state"
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
	t.Run("unscoped", func(t *testing.T) {
		dir := failedWaiters(t)
		if _, err := endTurn(t, boundaryEnd{failed: true, text: "verbatim failure"}); err != nil {
			t.Fatal(err)
		}
		check(t, dir)
	})
	t.Run("completion", func(t *testing.T) {
		dir := failedWaiters(t)
		self, _ := registry.Lookup(dir, "api")
		if err := completeTurn(dir, self, mail.TurnEnd{Boundary: boundaryNow(t, dir, self), ID: "failed", Failed: true, Text: "verbatim failure"}, ""); err != nil {
			t.Fatal(err)
		}
		check(t, dir)
	})
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
			if _, err := endTurn(t, boundaryEnd{failed: true, text: "failure"}); err != nil {
				t.Fatal(err)
			}
			if self {
				messages, err := mail.PeekUnread(dir, "api", epochOf(t, dir, "api"))
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

func TestReadingAnErrorOwesNothing(t *testing.T) {
	dir := liveSession(t, "api")
	peer := otherRun(t, dir, "web")
	t.Setenv(state.SessionEnv, "api")
	rawUnread(t, dir, "api", map[string]any{"kind": "error", "text": "failure", "from": "web", "fromEpoch": peer.Epoch(), "toEpoch": epochOf(t, dir, "api")})
	run("inbox")
	if len(mail.Waiters(dir, "api", epochOf(t, dir, "api"))) != 0 {
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
	event := mail.TurnEnd{Boundary: boundaryNow(t, dir, self), ID: "failure-once", Failed: true, Text: "failure"}
	for range 2 {
		if err := completeTurn(dir, self, event, ""); err != nil {
			t.Fatal(err)
		}
	}
	if len(finishedFor(t, dir, "web")) != 1 || len(finishedFor(t, dir, "leader")) != 0 {
		t.Fatal("a repeated end rerouted an already reported failure")
	}
}

func TestQuestionsReturnAnErrorOutcome(t *testing.T) {
	dir, peer := questionSender(t)
	awaitStatus = func(_, _, id string, _ time.Duration, _ func() bool) (mail.Status, bool) {
		rawUnread(t, dir, "web", map[string]any{"kind": "error", "text": "failure", "toEpoch": peer.Epoch(), "inReplyTo": []string{id}})
		return mail.Status{State: mail.Delivered}, true
	}
	code, out, errOut := run("send", "api", "question", "--question", "--wait", "0.2", "--json")
	if code != ExitFailed || !strings.Contains(out, `"kind": "error"`) || !strings.Contains(out, `"answer": "failure"`) {
		t.Fatalf("question=%d %s %s", code, out, errOut)
	}
}
