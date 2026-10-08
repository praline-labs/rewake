package cli

import (
	"context"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/harness"

	"github.com/praline-labs/rewake/internal/harness/codex/gateway"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
)

func reviewNativeEvent(t *testing.T, ui, native *integrationWire, method string, params any) {
	t.Helper()
	if err := native.write(map[string]any{"method": method, "params": params}); err != nil {
		t.Fatal(err)
	}
	if _, err := ui.read(); err != nil {
		t.Fatal(err)
	}
}

func reviewOutcome(t *testing.T, completed <-chan gateway.Completion) gateway.Completion {
	t.Helper()
	select {
	case result := <-completed:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("gateway did not publish outcome")
		return gateway.Completion{}
	}
}

func reviewPublish(t *testing.T, dir string, self registry.Session, outcome gateway.Completion) {
	t.Helper()
	// Use the same canonical identity and reporting entry point as the adapter.
	clock, err := inbox.OpenReadClock(context.Background(), dir, self.Name, self.Epoch())
	if err != nil {
		t.Fatal(err)
	}
	defer clock.Close()
	err = ReportCompletion(context.Background(), dir, self, harness.Completion{ID: outcome.PublicationID(), Text: outcome.Text, Thread: outcome.Thread, Kind: inbox.Kind(outcome.Kind), Boundary: clock.Snapshot(), Started: outcome.Started, Ended: outcome.Ended})
	if err != nil {
		t.Fatal(err)
	}
}

func TestReviewScopedGapReceiptsRemainDistinct(t *testing.T) {
	dir := liveSession(t, "api")
	peer := otherRun(t, dir, "web")
	self, _ := registry.Lookup(dir, "api")
	completed := make(chan gateway.Completion, 4)
	_, ui, native := gatewayWireFixture(t, self.Epoch(), func(result gateway.Completion) { completed <- result })
	wireExchange(t, ui, native, 1, "thread/start", map[string]any{"threadSource": "user", "runtimeWorkspaceRoots": []string{}}, map[string]any{"thread": map[string]any{"id": "A", "canAcceptDirectInput": true}})
	var outcomes []gateway.Completion
	for i := range 2 {
		if i > 0 {
			wireExchange(t, ui, native, 2, "thread/resume", map[string]any{"threadId": "A", "config": map[string]any{}, "runtimeWorkspaceRoots": []string{}}, map[string]any{"thread": map[string]any{"id": "A", "canAcceptDirectInput": true}})
			wireExchange(t, ui, native, 3, "thread/goal/get", map[string]string{"threadId": "A"}, map[string]any{})
		}
		readFrom(t, dir, peer)
		for _, status := range []string{"active", "idle"} {
			reviewNativeEvent(t, ui, native, "thread/status/changed", map[string]any{"threadId": "A", "status": map[string]string{"type": status}})
		}
		outcome := reviewOutcome(t, completed)
		outcomes = append(outcomes, outcome)
		reviewPublish(t, dir, self, outcome)
	}
	waiters := inbox.Waiters(dir, self.Name, self.Epoch())
	t.Logf("gateway outcomes=%+v reports=%v remaining=%v", outcomes, finishedFor(t, dir, peer.Name), waiters)
	if len(finishedFor(t, dir, peer.Name)) != 2 {
		t.Fatal("distinct admission generations collapsed onto one durable gap receipt")
	}
	// A gap is advisory: it settles nothing, and the wait stays.
	if len(waiters) != 1 {
		t.Fatal("a gap settled the wait")
	}
}

func TestReviewStoppedReceiptAllowsSameTurnFinal(t *testing.T) {
	dir := liveSession(t, "api")
	peer := otherRun(t, dir, "web")
	self, _ := registry.Lookup(dir, "api")
	readFrom(t, dir, peer)
	completed := make(chan gateway.Completion, 4)
	_, ui, native := gatewayWireFixture(t, self.Epoch(), func(result gateway.Completion) { completed <- result })
	wireExchange(t, ui, native, 1, "thread/start", map[string]any{"threadSource": "user", "runtimeWorkspaceRoots": []string{}}, map[string]any{"thread": map[string]any{"id": "A", "canAcceptDirectInput": true}})
	wireExchange(t, ui, native, 2, "turn/start", map[string]string{"threadId": "A"}, map[string]any{"turn": map[string]string{"id": "T"}})
	reviewNativeEvent(t, ui, native, "turn/started", map[string]any{"threadId": "A", "turn": map[string]string{"id": "T"}})
	reviewNativeEvent(t, ui, native, "turn/completed", map[string]any{"threadId": "A", "turn": map[string]string{"id": "T", "status": "interrupted"}})
	stopped := reviewOutcome(t, completed)
	reviewPublish(t, dir, self, stopped)
	reviewNativeEvent(t, ui, native, "item/completed", map[string]any{"threadId": "A", "turnId": "T", "item": map[string]string{"type": "agentMessage", "text": "continued result"}})
	reviewNativeEvent(t, ui, native, "turn/completed", map[string]any{"threadId": "A", "turn": map[string]string{"id": "T", "status": "completed"}})
	final := reviewOutcome(t, completed)
	if stopped.Kind != "stopped" || final.Kind != "finished" {
		t.Fatal("invalid reproduction outcomes", stopped, final)
	}
	reviewPublish(t, dir, self, final)
	waiters := inbox.Waiters(dir, self.Name, self.Epoch())
	t.Logf("stopped=%+v final=%+v reports=%v remaining=%v", stopped, final, finishedFor(t, dir, peer.Name), waiters)
	if len(finishedFor(t, dir, peer.Name)) != 2 || len(waiters) != 0 {
		t.Fatal("advisory stopped receipt suppressed the same turn's final report")
	}
}

func TestStoppedAndErrorRetriesSettleOriginalScopeOnce(t *testing.T) {
	dir := liveSession(t, "api")
	peer := otherRun(t, dir, "web")
	self, _ := registry.Lookup(dir, "api")
	first := readFrom(t, dir, peer)
	clock, err := inbox.OpenReadClock(context.Background(), dir, self.Name, self.Epoch())
	if err != nil {
		t.Fatal(err)
	}
	defer clock.Close()
	stopped := harness.Completion{ID: "A/T", Thread: "A", Kind: inbox.Stopped, Text: "stopped", Boundary: clock.Snapshot()}
	final := stopped
	final.Kind = inbox.Error
	final.Text = "final error"
	// Both events were observed before the second message, but publication is delayed.
	second := readFrom(t, dir, peer)
	for _, event := range []harness.Completion{stopped, stopped, final, final} {
		if err := ReportCompletion(context.Background(), dir, self, event); err != nil {
			t.Fatal(err)
		}
	}
	reports := finishedFor(t, dir, peer.Name)
	waiters := inbox.Waiters(dir, self.Name, self.Epoch())
	if len(reports) != 2 || len(waiters) != 1 || len(waiters[0].Messages) != 1 || waiters[0].Messages[0] != second {
		t.Fatalf("first=%s later=%s reports=%v remaining=%v", first, second, reports, waiters)
	}
}
