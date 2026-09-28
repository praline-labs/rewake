package cli

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/harness/codex/gateway"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
)

// Owner contract: the primary keeps working while a side answers; its answer
// must neither take the address nor consume the primary task's durable wait.
func testSideWhilePrimaryWorks(t *testing.T, dir string, self, sender registry.Session, g *gateway.Gateway, ui, native *integrationWire, completed <-chan gateway.Completion) {
	t.Helper()
	emit := func(method string, params any) {
		t.Helper()
		if err := native.write(map[string]any{"method": method, "params": params}); err != nil {
			t.Fatal(err)
		}
		if _, err := ui.read(); err != nil {
			t.Fatal(err)
		}
	}
	emit("turn/started", map[string]any{"threadId": "A", "turn": map[string]string{"id": "work"}})
	wireExchange(t, ui, native, 2, "thread/fork", map[string]any{"threadId": "A", "threadSource": "user", "config": map[string]any{}, "runtimeWorkspaceRoots": []string{}}, map[string]any{"thread": map[string]any{"id": "side", "canAcceptDirectInput": true}})
	wireExchange(t, ui, native, 3, "thread/inject_items", map[string]any{"threadId": "side", "items": []any{}}, map[string]any{})
	if b := g.Binding(); !b.Ready || b.Thread != "A" {
		t.Fatal("side setup lost primary", b)
	}
	wireExchange(t, ui, native, 4, "turn/start", map[string]string{"threadId": "side"}, map[string]any{"turn": map[string]string{"id": "side-work"}})
	emit("turn/started", map[string]any{"threadId": "side", "turn": map[string]string{"id": "side-work"}})
	emit("item/completed", map[string]any{"threadId": "side", "turnId": "side-work", "item": map[string]string{"type": "agentMessage", "text": "side answer"}})
	emit("turn/completed", map[string]any{"threadId": "side", "turn": map[string]string{"id": "side-work", "status": "completed"}})
	select {
	case result := <-completed:
		t.Fatalf("side settled main work: %+v", result)
	default:
	}
	if waits := inbox.Waiters(dir, self.Name, self.Epoch()); len(waits) != 1 {
		t.Fatalf("main wait changed: %v", waits)
	}
	note := inbox.Message{ID: inbox.NewID(), From: sender.Name, FromEpoch: sender.Epoch(), To: self.Name, ToEpoch: self.Epoch(), Kind: inbox.Note, Text: "keep working in main", CreatedAt: time.Now()}
	if err := inbox.Put(dir, note); err != nil {
		t.Fatal(err)
	}
	request, err := native.read()
	if err != nil {
		t.Fatal(err)
	}
	var params struct {
		Thread string `json:"threadId"`
	}
	_ = json.Unmarshal(request["params"], &params)
	if params.Thread != "A" {
		t.Fatal("new rewake message addressed side")
	}
	if err := native.write(map[string]any{"id": request["id"], "result": map[string]any{"turn": map[string]string{"id": "work"}}}); err != nil {
		t.Fatal(err)
	}
	wireNoticeDisplay(t, ui, "A", "work")
	waitIntegration(t, func() bool {
		status, ok := inbox.ReadStatus(dir, self.Name, note.ID)
		return ok && status.State == inbox.Delivered
	})
}
