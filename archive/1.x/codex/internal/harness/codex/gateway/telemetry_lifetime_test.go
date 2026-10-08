package gateway

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestTelemetryStagesPreAckAndCountsAcrossReconnectAndFork(t *testing.T) {
	g, ui, peers, path := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	write(t, ui, []byte(startA))
	_ = readWithin(t, native)
	telemetryEvent(t, ui, native, `{"method":"thread/settings/updated","params":{"threadId":"A","threadSettings":{"model":"notified","effort":"high"}}}`)
	telemetryEvent(t, ui, native, usageEvent("A", "T", "121200", "272000"))
	telemetryEvent(t, ui, native, compactionEvent("A", "early", true))
	if g.SessionState().Fresh {
		t.Fatal("unacknowledged primary telemetry published as fresh")
	}
	telemetryEvent(t, ui, native, `{"id":1,"result":{"thread":{"id":"A","canAcceptDirectInput":true},"model":"first"}}`)
	s := assertCompactions(t, g, 1)
	if s.FilledPercent == nil || *s.FilledPercent != 42 || s.Model == nil || *s.Model != "notified" {
		t.Fatal("pre-ACK usage lost")
	}
	old := g.currentConnection()
	_ = ui.conn.Close()
	select {
	case <-old.cleaned:
	case <-time.After(time.Second):
		t.Fatal("not disconnected")
	}
	s = assertCompactions(t, g, 1)
	if s.Fresh || s.Coverage != "partial" {
		t.Fatal("disconnect did not invalidate freshness/coverage")
	}
	next, err := dialSocket(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = next.conn.Close() }()
	resumed := <-peers
	defer func() { _ = resumed.conn.Close() }()
	exchange(t, next, resumed, `{"id":0,"method":"initialize"}`, `{"id":0,"result":{}}`)
	exchange(t, next, resumed, `{"id":2,"method":"thread/resume","params":{"threadId":"A","config":{},"runtimeWorkspaceRoots":[]}}`, `{"id":2,"result":{"thread":{"id":"A","canAcceptDirectInput":true},"model":"first","reasoningEffort":null}}`)
	s = assertCompactions(t, g, 1)
	if s.ContextUsed != nil || s.Effort != nil {
		t.Fatal("reconnect invented state")
	}
	telemetryEvent(t, next, resumed, compactionEvent("A", "early", true))
	assertCompactions(t, g, 1)
	write(t, next, []byte(`{"id":3,"method":"thread/fork","params":{"threadId":"A","threadSource":"user","config":{},"runtimeWorkspaceRoots":[]}}`))
	_ = readWithin(t, resumed)
	telemetryEvent(t, next, resumed, compactionEvent("child", "new-fork-item", true))
	assertCompactions(t, g, 1)
	telemetryEvent(t, next, resumed, `{"id":3,"result":{"thread":{"id":"child","canAcceptDirectInput":true}}}`)
	telemetryEvent(t, next, resumed, compactionEvent("A", "parent-before-detach", true))
	exchange(t, next, resumed, `{"id":4,"method":"thread/unsubscribe","params":{"threadId":"A"}}`, `{"id":4,"result":{"status":"unsubscribed"}}`)
	s = assertCompactions(t, g, 3)
	if s.Thread != "child" || s.Coverage != "partial" {
		t.Fatal("fork reset counter or erased coverage gap")
	}
	fresh := New(Config{Epoch: "new-wrapper"})
	if count := fresh.SessionState().Compactions; count == nil || *count != 0 {
		t.Fatal("new wrapper did not reset observed count")
	}
}

func TestTelemetryBoundDoesNotEvictCompletedIDs(t *testing.T) {
	run := telemetryRun{}
	entry := threadObservation{}
	for i := 0; i < maxCompactionIDs+1; i++ {
		run.compaction(&entry, compactionObservation{thread: "A", item: fmt.Sprint(i), turn: "T", completed: true})
	}
	run.compaction(&entry, compactionObservation{thread: "A", item: "0", turn: "T", completed: true})
	if run.count != maxCompactionIDs || len(run.seen) != maxCompactionIDs || !run.partial {
		t.Fatal("bounded dedup silently recounted or claimed full coverage")
	}
}

func TestConfirmedMetadataRefreshAndUnknownSelection(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	modelBinding(t, g, ui, native)
	exchange(t, ui, native, `{"id":2,"method":"thread/settings/update","params":{"threadId":"A","model":"proposed"}}`, `{"id":2,"result":{}}`)
	exchange(t, ui, native, `{"id":3,"method":"thread/read","params":{"threadId":"A","includeTurns":false}}`, `{"id":3,"result":{"thread":{"id":"A","status":{"type":"idle"},"model":"loaded","reasoningEffort":"high"}}}`)
	s := g.SessionState()
	if s.Model == nil || *s.Model != "loaded" || !s.SettingsFresh {
		t.Fatal("metadata-only confirmed state not applied")
	}
	exchange(t, ui, native, `{"id":4,"method":"thread/read","params":{"threadId":"other","includeTurns":true}}`, `{"id":4,"result":{"thread":{"id":"other","model":"unrelated"}}}`)
	s = g.SessionState()
	if s.Fresh || s.SettingsFresh || s.Coverage != "partial" || *s.Model != "loaded" {
		t.Fatal("unknown read selected or refreshed telemetry")
	}
}
