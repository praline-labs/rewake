package gateway

import (
	"fmt"
	"testing"
)

func metadataRequest(t *testing.T, ui, native *socketClient, id int) {
	t.Helper()
	write(t, ui, []byte(fmt.Sprintf(`{"id":%d,"method":"thread/read","params":{"threadId":"A","includeTurns":false}}`, id)))
	_ = readWithin(t, native)
}

func metadataReply(t *testing.T, ui, native *socketClient, id int, model, effort, status string) {
	t.Helper()
	telemetryEvent(t, ui, native, fmt.Sprintf(`{"id":%d,"result":{"thread":{"id":"A","status":{"type":%q},"model":%q,"reasoningEffort":%q}}}`, id, status, model, effort))
}

func assertSettings(t *testing.T, g *Gateway, model, effort string, fresh bool) {
	t.Helper()
	s := g.SessionState()
	if s.Model == nil || *s.Model != model || s.Effort == nil || *s.Effort != effort || s.SettingsFresh != fresh {
		t.Fatalf("settings: model=%v effort=%v fresh=%v; want %s/%s fresh=%v", s.Model, s.Effort, s.SettingsFresh, model, effort, fresh)
	}
}

func TestConcurrentMetadataOrdersAndSettingsFences(t *testing.T) {
	for _, intervention := range []string{"none", "notification", "rejection", "selection"} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%v", intervention, reverse), func(t *testing.T) {
				g, ui, peers, _ := setup(t)
				native := <-peers
				defer func() { _ = native.conn.Close() }()
				modelBinding(t, g, ui, native)
				// Request order is independent of numeric RPC ID order.
				metadataRequest(t, ui, native, 20)
				metadataRequest(t, ui, native, 10)
				first, last := 20, 10
				firstModel, firstEffort := "older", "low"
				lastModel, lastEffort := "newer", "high"
				if reverse {
					first, last = 10, 20
					firstModel, firstEffort, lastModel, lastEffort = "newer", "high", "older", "low"
				}
				metadataReply(t, ui, native, first, firstModel, firstEffort, "idle")
				expected, effort := "newer", "high"
				switch intervention {
				case "notification":
					telemetryEvent(t, ui, native, `{"method":"thread/settings/updated","params":{"threadId":"A","threadSettings":{"model":"applied","effort":"high"}}}`)
					expected = "applied"
				case "rejection":
					exchange(t, ui, native, `{"id":30,"method":"thread/settings/update","params":{"threadId":"A","model":"invalid"}}`, `{"id":30,"error":{"code":-32602}}`)
					expected, effort = firstModel, firstEffort
				case "selection":
					exchange(t, ui, native, `{"id":30,"method":"thread/resume","params":{"threadId":"A","config":{},"runtimeWorkspaceRoots":[]}}`, `{"id":30,"result":{"thread":{"id":"A","canAcceptDirectInput":true},"model":"selected","reasoningEffort":"high"}}`)
					expected = "selected"
				}
				metadataReply(t, ui, native, last, lastModel, lastEffort, "idle")
				assertSettings(t, g, expected, effort, true)
				metadataRequest(t, ui, native, 40)
				metadataReply(t, ui, native, 40, "latest", "high", "idle")
				assertSettings(t, g, "latest", "high", true)
			})
		}
	}
}

func TestLifecycleConfirmationSeparatesProposalFromAppliedEvidence(t *testing.T) {
	for _, method := range []string{"thread/start", "thread/resume"} {
		for _, scenario := range []string{"rejected-before", "pending-at-ack", "applied-before"} {
			t.Run(method+"/"+scenario, func(t *testing.T) {
				g, ui, peers, _ := setup(t)
				native := <-peers
				defer func() { _ = native.conn.Close() }()
				write(t, ui, []byte(fmt.Sprintf(`{"id":1,"method":%q,"params":{"threadId":"A","threadSource":"user","config":{},"runtimeWorkspaceRoots":[]}}`, method)))
				_ = readWithin(t, native)
				if scenario == "applied-before" {
					telemetryEvent(t, ui, native, `{"method":"thread/settings/updated","params":{"threadId":"A","threadSettings":{"model":"applied","effort":"high"}}}`)
				}
				write(t, ui, []byte(`{"id":2,"method":"thread/settings/update","params":{"threadId":"A","model":"invalid"}}`))
				_ = readWithin(t, native)
				if scenario != "pending-at-ack" {
					telemetryEvent(t, ui, native, `{"id":2,"error":{"code":-32602}}`)
				}
				telemetryEvent(t, ui, native, `{"id":1,"result":{"thread":{"id":"A","canAcceptDirectInput":true},"model":"confirmed","reasoningEffort":"high"}}`)
				expected := "confirmed"
				if scenario == "applied-before" {
					expected = "applied"
				}
				assertSettings(t, g, expected, "high", scenario != "pending-at-ack")
				if scenario == "pending-at-ack" {
					telemetryEvent(t, ui, native, `{"id":2,"error":{"code":-32602}}`)
					assertSettings(t, g, "confirmed", "high", true)
				}
				telemetryEvent(t, ui, native, `{"method":"turn/started","params":{"threadId":"A","turn":{"id":"new"}}}`)
				telemetryEvent(t, ui, native, usageEvent("A", "new", "122000", "272000"))
				if !g.SessionState().ContextFresh {
					t.Fatal("valid usage did not recover")
				}
			})
		}
	}
}

func TestUnloadedMetadataPreservesRequestOrdering(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprint(reverse), func(t *testing.T) {
			g, ui, peers, _ := setup(t)
			native := <-peers
			defer func() { _ = native.conn.Close() }()
			modelBinding(t, g, ui, native)
			metadataRequest(t, ui, native, 2)
			metadataRequest(t, ui, native, 3)
			if reverse {
				metadataReply(t, ui, native, 3, "cached", "low", "notLoaded")
				metadataReply(t, ui, native, 2, "obsolete", "low", "idle")
				assertSettings(t, g, "first", "low", false)
			} else {
				metadataReply(t, ui, native, 2, "cached", "low", "notLoaded")
				metadataReply(t, ui, native, 3, "current", "high", "idle")
				assertSettings(t, g, "current", "high", true)
			}
		})
	}
}

func TestLifecycleSeedDoesNotRewindNewerReadWatermark(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	write(t, ui, []byte(`{"id":1,"method":"thread/resume","params":{"threadId":"A","config":{},"runtimeWorkspaceRoots":[]}}`))
	_ = readWithin(t, native)
	// Pre-ACK overview reads use UUID IDs; numeric reads would invalidate selection.
	older, newer := "00000000-0000-4000-8000-000000000002", "00000000-0000-4000-8000-000000000003"
	for _, id := range []string{older, newer} {
		write(t, ui, []byte(fmt.Sprintf(`{"id":%q,"method":"thread/read","params":{"threadId":"A","includeTurns":false}}`, id)))
		_ = readWithin(t, native)
	}
	telemetryEvent(t, ui, native, fmt.Sprintf(`{"id":%q,"result":{"thread":{"id":"A","status":{"type":"notLoaded"},"model":"cached","reasoningEffort":"low"}}}`, newer))
	telemetryEvent(t, ui, native, `{"id":1,"result":{"thread":{"id":"A","canAcceptDirectInput":true},"model":"confirmed","reasoningEffort":"high"}}`)
	telemetryEvent(t, ui, native, fmt.Sprintf(`{"id":%q,"result":{"thread":{"id":"A","status":{"type":"idle"},"model":"obsolete","reasoningEffort":"low"}}}`, older))
	assertSettings(t, g, "confirmed", "high", true)
}
