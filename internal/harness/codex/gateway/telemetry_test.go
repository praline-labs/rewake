package gateway

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/praline-labs/rewake/internal/sessionstate"
)

func number(value int64) *int64 { return &value }

func TestContextFillUsesLatestNativeFormulaAndUnknowns(t *testing.T) {
	for _, test := range []struct {
		used, window *int64
		want         int
	}{
		{number(-1), number(272000), -1},
		{number(10), number(-1), -1},
		{nil, number(272000), -1},
		{number(1), nil, -1},
		{number(0), number(272000), 0},
		{number(12000), number(272000), 0},
		{number(121200), number(272000), 42},
		{number(142000), number(272000), 50},
		{number(1000000), number(272000), 100},
		{number(1), number(12000), 100},
		{number(13300), number(272000), 0},
	} {
		got := contextFill(test.used, test.window)
		if test.want < 0 && got != nil || test.want >= 0 && (got == nil || *got != test.want) {
			t.Fatalf("fill=%v want=%d", got, test.want)
		}
	}
	for _, raw := range []string{"", "null", "-1", "1.2", "true", `"42"`, "9223372036854775808"} {
		if telemetryInteger([]byte(raw)) != nil {
			t.Fatalf("invented integer from %q", raw)
		}
	}
}

func telemetryEvent(t *testing.T, ui, native *socketClient, raw string) {
	t.Helper()
	write(t, native, []byte(raw))
	if !bytes.Equal(readWithin(t, ui), []byte(raw)) {
		t.Fatal("observation changed native payload")
	}
}

func usageEvent(thread, turn, used, window string) string {
	return fmt.Sprintf(`{"method":"thread/tokenUsage/updated","params":{"threadId":%q,"turnId":%q,"tokenUsage":{"last":{"totalTokens":%s},"total":{"totalTokens":999999999},"modelContextWindow":%s}}}`, thread, turn, used, window)
}

func compactionEvent(thread, item string, completed bool) string {
	method := "item/started"
	if completed {
		method = "item/completed"
	}
	return fmt.Sprintf(`{"method":%q,"params":{"threadId":%q,"turnId":"T","item":{"type":"contextCompaction","id":%q}}}`, method, thread, item)
}

func modelBinding(t *testing.T, g *Gateway, ui, native *socketClient) {
	t.Helper()
	exchange(t, ui, native, startA, `{"id":1,"result":{"thread":{"id":"A","canAcceptDirectInput":true},"model":"first","reasoningEffort":"low"}}`)
	snapshot := g.SessionState()
	if snapshot.Model == nil || *snapshot.Model != "first" || snapshot.ContextUsed != nil || !snapshot.SettingsFresh {
		t.Fatalf("initial state: %+v", snapshot)
	}
}

func TestAppliedSettingsReplaceLabelsWithoutRelabelingOldUsage(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	modelBinding(t, g, ui, native)
	telemetryEvent(t, ui, native, usageEvent("A", "old", "121200", "272000"))
	s := g.SessionState()
	if s.FilledPercent == nil || *s.FilledPercent != 42 || *s.ContextUsed != 121200 {
		t.Fatalf("usage %+v", s)
	}
	exchange(t, ui, native, `{"id":2,"method":"thread/settings/update","params":{"threadId":"A","model":"proposed","effort":"high"}}`, `{"id":2,"result":{}}`)
	s = g.SessionState()
	if *s.Model != "first" || s.SettingsFresh || s.ContextFresh {
		t.Fatal("empty ACK was presented as applied settings")
	}
	telemetryEvent(t, ui, native, `{"method":"thread/settings/updated","params":{"threadId":"A","threadSettings":{"model":"confirmed","effort":null}}}`)
	s = g.SessionState()
	if *s.Model != "confirmed" || s.Effort != nil || s.ContextWindow != nil || !s.SettingsFresh {
		t.Fatalf("applied state %+v", s)
	}
	telemetryEvent(t, ui, native, usageEvent("A", "old", "90000", "272000"))
	if g.SessionState().ContextWindow != nil {
		t.Fatal("old active turn usage relabeled with new model")
	}
	telemetryEvent(t, ui, native, `{"method":"turn/started","params":{"threadId":"A","turn":{"id":"new"}}}`)
	telemetryEvent(t, ui, native, usageEvent("A", "new", "92000", "512123"))
	s = g.SessionState()
	if s.ContextWindow == nil || *s.ContextWindow != 512123 || !s.ContextFresh {
		t.Fatal("new turn did not refresh usage")
	}
	telemetryEvent(t, ui, native, usageEvent("A", "new", "-1", "null"))
	s = g.SessionState()
	if s.ContextUsed != nil || s.ContextWindow != nil || s.FilledPercent != nil {
		t.Fatal("invalid usage was not unknown")
	}
}

func assertCompactions(t *testing.T, g *Gateway, count uint64) sessionstate.Snapshot {
	t.Helper()
	s := g.SessionState()
	if s.Compactions == nil || *s.Compactions != count {
		t.Fatalf("compactions=%v want=%d", s.Compactions, count)
	}
	return s
}

func TestPrimaryCompactionsExcludeSideHistoryAndDeduplicateAcrossSelection(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	modelBinding(t, g, ui, native)
	exchange(t, ui, native, `{"id":10,"method":"thread/compact/start","params":{"threadId":"A"}}`, `{"id":10,"result":{}}`)
	telemetryEvent(t, ui, native, compactionEvent("A", "manual", false))
	s := assertCompactions(t, g, 0)
	if s.Compacting == nil || !*s.Compacting {
		t.Fatal("start not distinguishable from completion")
	}
	telemetryEvent(t, ui, native, compactionEvent("A", "manual", true))
	telemetryEvent(t, ui, native, compactionEvent("A", "manual", true))
	telemetryEvent(t, ui, native, compactionEvent("nested", "ignored", true))
	telemetryEvent(t, ui, native, `{"method":"thread/compacted","params":{"threadId":"A"}}`)
	telemetryEvent(t, ui, native, `{"method":"turn/completed","params":{"threadId":"A","turn":{"id":"T","status":"completed"}}}`)
	assertCompactions(t, g, 1)
	telemetryEvent(t, ui, native, usageEvent("A", "T", "121200", "272000"))
	forkPair(t, ui, native, 2, "A", "side")
	if g.SessionState().Fresh {
		t.Fatal("unresolved side fork looked freshly selected")
	}
	telemetryEvent(t, ui, native, compactionEvent("side", "side-staged", true))
	telemetryEvent(t, ui, native, compactionEvent("A", "during-side-setup", true))
	exchange(t, ui, native, `{"id":3,"method":"thread/inject_items","params":{"threadId":"side","items":[]}}`, `{"id":3,"result":{}}`)
	s = assertCompactions(t, g, 2)
	if s.Thread != "A" || s.FilledPercent == nil || *s.FilledPercent != 42 {
		t.Fatal("side changed primary observations")
	}
	telemetryEvent(t, ui, native, compactionEvent("side", "side-live", true))
	telemetryEvent(t, ui, native, `{"method":"thread/settings/updated","params":{"threadId":"side","threadSettings":{"model":"side-model","effort":"high"}}}`)
	telemetryEvent(t, ui, native, usageEvent("side", "T", "999", "1000"))
	s = assertCompactions(t, g, 2)
	if *s.Model != "first" || *s.ContextWindow != 272000 {
		t.Fatal("side state replaced primary")
	}
	exchange(t, ui, native, `{"id":4,"method":"thread/unsubscribe","params":{"threadId":"side"}}`, `{"id":4,"result":{"status":"unsubscribed"}}`)
	telemetryEvent(t, ui, native, compactionEvent("A", "automatic", true))
	assertCompactions(t, g, 3)
	exchange(t, ui, native, `{"id":5,"method":"thread/start","params":{"threadSource":"user","runtimeWorkspaceRoots":[]}}`, `{"id":5,"result":{"thread":{"id":"B","canAcceptDirectInput":true},"model":"second"}}`)
	s = assertCompactions(t, g, 3)
	if s.ContextUsed != nil || *s.Model != "second" {
		t.Fatal("new conversation inherited stale context")
	}
	exchange(t, ui, native, `{"id":6,"method":"thread/resume","params":{"threadId":"A","config":{},"runtimeWorkspaceRoots":[]}}`, `{"id":6,"result":{"thread":{"id":"A","canAcceptDirectInput":true,"turns":[{"items":[{"type":"contextCompaction","id":"history"}]}]},"model":"first"}}`)
	assertCompactions(t, g, 3)
	telemetryEvent(t, ui, native, compactionEvent("A", "manual", true))
	assertCompactions(t, g, 3)
	telemetryEvent(t, ui, native, usageEvent("A", "T", "13000", "272000"))
	if g.SessionState().ContextUsed == nil {
		t.Fatal("restored usage snapshot ignored")
	}
}
