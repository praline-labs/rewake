package gateway

import (
	"fmt"
	"testing"
)

func runningUsage(t *testing.T, ui, native *socketClient, turn string, used int) {
	t.Helper()
	telemetryEvent(t, ui, native, fmt.Sprintf(`{"method":"turn/started","params":{"threadId":"A","turn":{"id":%q}}}`, turn))
	telemetryEvent(t, ui, native, usageEvent("A", turn, fmt.Sprint(used), "272000"))
}

func TestRejectedProposalRecoversCurrentUsageAcrossLifecycleOrders(t *testing.T) {
	for _, method := range []string{"thread/start", "thread/resume"} {
		for _, order := range []string{"turn-proposal-ack-reject", "proposal-turn-ack-reject", "proposal-ack-turn-reject", "proposal-turn-reject-ack"} {
			t.Run(method+"/"+order, func(t *testing.T) {
				g, ui, peers, _ := setup(t)
				native := <-peers
				defer func() { _ = native.conn.Close() }()
				write(t, ui, []byte(fmt.Sprintf(`{"id":1,"method":%q,"params":{"threadId":"A","threadSource":"user","config":{},"runtimeWorkspaceRoots":[]}}`, method)))
				_ = readWithin(t, native)
				if order == "turn-proposal-ack-reject" {
					runningUsage(t, ui, native, "running", 121200)
				}
				write(t, ui, []byte(`{"id":2,"method":"thread/settings/update","params":{"threadId":"A","model":"invalid"}}`))
				_ = readWithin(t, native)
				if order == "proposal-turn-ack-reject" || order == "proposal-turn-reject-ack" {
					runningUsage(t, ui, native, "running", 121200)
				}
				if order == "proposal-turn-reject-ack" {
					telemetryEvent(t, ui, native, `{"id":2,"error":{"code":-32602}}`)
				}
				telemetryEvent(t, ui, native, `{"id":1,"result":{"thread":{"id":"A","canAcceptDirectInput":true},"model":"confirmed","reasoningEffort":"high"}}`)
				if order == "proposal-ack-turn-reject" {
					runningUsage(t, ui, native, "running", 121200)
				}
				if order != "proposal-turn-reject-ack" {
					if g.SessionState().SettingsFresh {
						t.Fatal("lifecycle seed settled the pending proposal")
					}
					telemetryEvent(t, ui, native, `{"id":2,"error":{"code":-32602}}`)
				}
				telemetryEvent(t, ui, native, usageEvent("A", "running", "122000", "272000"))
				s := g.SessionState()
				if s.Model == nil || *s.Model != "confirmed" || !s.SettingsFresh || !s.ContextFresh || s.ContextUsed == nil || *s.ContextUsed != 122000 {
					t.Fatalf("current-turn usage did not recover: %+v", s)
				}
			})
		}
	}
}

func TestAppliedSettingsBarrierSurvivesRejectionAroundLifecycle(t *testing.T) {
	for _, beforeACK := range []bool{false, true} {
		t.Run(fmt.Sprint(beforeACK), func(t *testing.T) {
			g, ui, peers, _ := setup(t)
			native := <-peers
			defer func() { _ = native.conn.Close() }()
			write(t, ui, []byte(`{"id":1,"method":"thread/resume","params":{"threadId":"A","config":{},"runtimeWorkspaceRoots":[]}}`))
			_ = readWithin(t, native)
			write(t, ui, []byte(`{"id":2,"method":"thread/settings/update","params":{"threadId":"A","model":"invalid"}}`))
			_ = readWithin(t, native)
			runningUsage(t, ui, native, "running", 121200)
			applied := `{"method":"thread/settings/updated","params":{"threadId":"A","threadSettings":{"model":"applied","effort":"high"}}}`
			if beforeACK {
				telemetryEvent(t, ui, native, applied)
			}
			telemetryEvent(t, ui, native, `{"id":1,"result":{"thread":{"id":"A","canAcceptDirectInput":true},"model":"confirmed","reasoningEffort":"high"}}`)
			if !beforeACK {
				telemetryEvent(t, ui, native, applied)
			}
			telemetryEvent(t, ui, native, `{"id":2,"error":{"code":-32602}}`)
			telemetryEvent(t, ui, native, usageEvent("A", "running", "122000", "272000"))
			s := g.SessionState()
			if s.Model == nil || *s.Model != "applied" || !s.SettingsFresh || s.ContextUsed != nil || s.ContextFresh {
				t.Fatal("old running usage crossed an actual applied-settings barrier")
			}
			runningUsage(t, ui, native, "new-turn", 123000)
			if s := g.SessionState(); !s.ContextFresh || s.ContextUsed == nil || *s.ContextUsed != 123000 {
				t.Fatal("new turn did not retire the applied-settings barrier")
			}
		})
	}
}
