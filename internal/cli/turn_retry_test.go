package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// closeMailbox leaves name's mailbox readable and closed to writes: a report
// into it fails as a plain failure a retry clears, while every mark it holds
// still reads. A file in its place would hide those marks, an unknown that
// stops the sender before any effect (rule 6).
func closeMailbox(t *testing.T, dir, name string) func() {
	t.Helper()
	mailbox := state.InboxPath(dir, name)
	if err := os.MkdirAll(mailbox, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(mailbox, 0o500); err != nil {
		t.Fatal(err)
	}
	unblock := func() { _ = os.Chmod(mailbox, 0o700) }
	t.Cleanup(unblock)
	return unblock
}

func TestRetriedTurnDoesNotConsumeLaterWork(t *testing.T) {
	dir := liveSession(t, "api")
	first := otherRun(t, dir, "one")
	second := otherRun(t, dir, "two")
	t.Setenv(state.SessionEnv, "api")
	t.Setenv(state.EpochEnv, epochOf(t, dir, "api"))
	rawUnread(t, dir, "api", map[string]any{"from": first.Name, "fromEpoch": first.Epoch(), "toEpoch": epochOf(t, dir, "api"), "text": "first task"})
	rawUnread(t, dir, "api", map[string]any{"from": second.Name, "fromEpoch": second.Epoch(), "toEpoch": epochOf(t, dir, "api"), "text": "second task"})
	run("inbox")
	unblock := closeMailbox(t, dir, "two")
	self, _ := registry.Lookup(dir, "api")
	event := inbox.TurnEnd{Boundary: boundaryNow(t, dir, self), ID: "turn-before-new-task", Failed: true, Text: "failure on original tasks"}
	if completeTurn(dir, self, event, "") == nil {
		t.Fatal("the blocked report did not fail")
	}
	if len(finishedFor(t, dir, "one")) != 1 {
		t.Fatal("first partial delivery did not happen")
	}
	late := rawUnread(t, dir, "api", map[string]any{"from": first.Name, "fromEpoch": first.Epoch(), "toEpoch": epochOf(t, dir, "api"), "kind": "task", "text": "task arriving after failed publication"})
	run("inbox")
	unblock()
	if err := completeTurn(dir, self, event, ""); err != nil {
		t.Fatal(err)
	}
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
	// Another outcome of the same turn is its own operation, and its scope is
	// the same boundary: the later work is not its to answer.
	changed := event
	changed.Failed, changed.Text = false, "different callback text"
	if err := completeTurn(dir, self, changed, ""); err != nil {
		t.Fatal(err)
	}
	if len(finishedFor(t, dir, "one")) != 1 || len(inbox.Waiters(dir, "api", epochOf(t, dir, "api"))) != 1 {
		t.Fatal("completed replay consumed later work")
	}
	if err := completeTurn(dir, self, inbox.TurnEnd{Boundary: boundaryNow(t, dir, self), ID: "next-turn", Text: "late task result"}, ""); err != nil {
		t.Fatal(err)
	}
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
	unblock := closeMailbox(t, dir, "web")
	self, _ := registry.Lookup(dir, "api")
	event := inbox.TurnEnd{Boundary: boundaryNow(t, dir, self), ID: "original", Failed: true, Text: "original failure"}
	if completeTurn(dir, self, event, "") == nil {
		t.Fatal("the blocked report did not fail")
	}
	late := readFrom(t, dir, leader)
	unblock()
	changed := event
	changed.Failed, changed.Text = false, "changed payload"
	if err := completeTurn(dir, self, changed, ""); err != nil {
		t.Fatal(err)
	}
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
