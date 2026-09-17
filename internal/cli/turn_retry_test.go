package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

func TestRetriedTurnDoesNotConsumeLaterWork(t *testing.T) {
	dir := liveSession(t, "api")
	first := otherRun(t, dir, "one")
	second := otherRun(t, dir, "two")
	t.Setenv(state.SessionEnv, "api")
	t.Setenv(state.EpochEnv, epochOf(t, dir, "api"))
	rawUnread(t, dir, "api", map[string]any{"from": first.Name, "fromEpoch": first.Epoch(), "toEpoch": epochOf(t, dir, "api"), "text": "first task"})
	rawUnread(t, dir, "api", map[string]any{"from": second.Name, "fromEpoch": second.Epoch(), "toEpoch": epochOf(t, dir, "api"), "text": "second task"})
	run("inbox")
	blocked := state.InboxPath(dir, "two")
	if err := os.WriteFile(blocked, []byte("temporarily unavailable mailbox"), 0o600); err != nil {
		t.Fatal(err)
	}
	payload := `{"type":"agent-turn-complete","turn-id":"turn-before-new-task","error":"failure on original tasks"}`
	run("turn-ended", payload)
	if len(finishedFor(t, dir, "one")) != 1 {
		t.Fatal("first partial delivery did not happen")
	}
	late := rawUnread(t, dir, "api", map[string]any{"from": first.Name, "fromEpoch": first.Epoch(), "toEpoch": epochOf(t, dir, "api"), "kind": "task", "text": "task arriving after failed publication"})
	run("inbox")
	if err := os.Remove(blocked); err != nil {
		t.Fatal(err)
	}
	run("turn-ended", payload)
	files := finishedFor(t, dir, "one")
	waits := inbox.Waiters(dir, "api", epochOf(t, dir, "api"))
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var m inbox.Message
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		t.Logf("report=%s replies=%v text=%q", filepath.Base(file), m.InReplyTo, m.Text)
	}
	t.Logf("late message=%s reports=%d remaining waits=%d", late, len(files), len(waits))
	if len(files) != 1 || len(waits) != 1 || !slices.Equal(waits[0].Messages, []string{late}) {
		t.Fatal("retry duplicated original report and consumed later work")
	}
	// A replay must use the stored outcome, not a changed callback payload.
	run("turn-ended", `{"type":"agent-turn-complete","turn-id":"turn-before-new-task","last-assistant-message":"different callback text"}`)
	if len(finishedFor(t, dir, "one")) != 1 || len(inbox.Waiters(dir, "api", epochOf(t, dir, "api"))) != 1 {
		t.Fatal("completed replay consumed later work")
	}
	run("turn-ended", `{"type":"agent-turn-complete","turn-id":"next-turn","last-assistant-message":"late task result"}`)
	files = finishedFor(t, dir, "one")
	if len(files) != 2 || len(inbox.Waiters(dir, "api", epochOf(t, dir, "api"))) != 0 {
		t.Fatal("next real result did not settle later work")
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var report inbox.Message
		if err := json.Unmarshal(raw, &report); err != nil {
			t.Fatal(err)
		}
		if slices.Contains(report.InReplyTo, late) && (report.Text != "late task result" || len(report.InReplyTo) != 1) {
			t.Fatalf("late task received an old result: %+v", report)
		}
	}
}

func TestPartialTurnRetryKeepsOriginalOutcome(t *testing.T) {
	dir := liveSession(t, "api")
	leader := otherRun(t, dir, "web")
	markMain(t, dir, "web")
	t.Setenv(state.SessionEnv, "api")
	t.Setenv(state.EpochEnv, epochOf(t, dir, "api"))
	blocked := state.InboxPath(dir, "web")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	run("turn-ended", `{"turn-id":"original","error":"original failure"}`)
	late := readFrom(t, dir, leader)
	if err := os.Remove(blocked); err != nil {
		t.Fatal(err)
	}
	run("turn-ended", `{"turn-id":"original","last_assistant_message":"changed payload"}`)
	files := finishedFor(t, dir, "web")
	if len(files) != 1 {
		t.Fatalf("reports = %d, want original fallback report", len(files))
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var report inbox.Message
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if report.Kind != inbox.Error || report.Text != "original failure" || len(report.InReplyTo) != 0 {
		t.Fatalf("retry changed the original outcome: %+v", report)
	}
	waits := inbox.Waiters(dir, "api", epochOf(t, dir, "api"))
	if len(waits) != 1 || !slices.Equal(waits[0].Messages, []string{late}) {
		t.Fatal("retry consumed work read after the original fallback")
	}
}
