package gateway

import "testing"

func TestReviewRejectedPreACKProposalKeepsAlreadyStartedTurnUsage(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	write(t, ui, []byte(`{"id":1,"method":"thread/resume","params":{"threadId":"A","config":{},"runtimeWorkspaceRoots":[]}}`))
	_ = readWithin(t, native)
	write(t, ui, []byte(`{"id":2,"method":"thread/settings/update","params":{"threadId":"A","model":"invalid"}}`))
	_ = readWithin(t, native)
	telemetryEvent(t, ui, native, `{"method":"turn/started","params":{"threadId":"A","turn":{"id":"running"}}}`)
	telemetryEvent(t, ui, native, usageEvent("A", "running", "121200", "272000"))
	telemetryEvent(t, ui, native, `{"id":1,"result":{"thread":{"id":"A","canAcceptDirectInput":true},"model":"confirmed","reasoningEffort":"high"}}`)
	telemetryEvent(t, ui, native, `{"id":2,"error":{"code":-32602}}`)
	// This is a new observation after rejection, not merely a cached pre-ACK sample.
	telemetryEvent(t, ui, native, usageEvent("A", "running", "122000", "272000"))
	s := g.SessionState()
	t.Logf("model=%v settingsFresh=%v contextUsed=%v contextFresh=%v", s.Model, s.SettingsFresh, s.ContextUsed, s.ContextFresh)
	telemetryEvent(t, ui, native, `{"method":"turn/started","params":{"threadId":"A","turn":{"id":"later"}}}`)
	telemetryEvent(t, ui, native, usageEvent("A", "later", "123000", "272000"))
	if recovered := g.SessionState(); !recovered.ContextFresh || recovered.ContextUsed == nil || *recovered.ContextUsed != 123000 {
		t.Fatal("control case: subsequent turn could not refresh context either")
	}
	t.Log("control case: a subsequent turn/start restores context observations")
	if s.Model == nil || *s.Model != "confirmed" || !s.SettingsFresh || s.ContextUsed == nil || *s.ContextUsed != 122000 || !s.ContextFresh {
		t.Fatal("lifecycle seed rearmed a gate that rejection cannot retire for the already-started turn")
	}
}
