package gateway

import "testing"

func TestReviewRejectedPreACKProposalKeepsLifecycleConfirmation(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	write(t, ui, []byte(`{"id":1,"method":"thread/resume","params":{"threadId":"A","config":{},"runtimeWorkspaceRoots":[]}}`))
	_ = readWithin(t, native)
	exchange(t, ui, native, `{"id":2,"method":"thread/settings/update","params":{"threadId":"A","model":"invalid"}}`, `{"id":2,"error":{"code":-32602}}`)
	telemetryEvent(t, ui, native, `{"id":1,"result":{"thread":{"id":"A","canAcceptDirectInput":true},"model":"confirmed","reasoningEffort":"high"}}`)
	s := g.SessionState()
	t.Logf("ready=%v model=%v effort=%v settingsFresh=%v", g.Binding().Ready, s.Model, s.Effort, s.SettingsFresh)
	if s.Model == nil || *s.Model != "confirmed" || s.Effort == nil || *s.Effort != "high" || !s.SettingsFresh {
		t.Fatal("rejected proposal suppressed the first valid lifecycle settings snapshot")
	}
}

func TestReviewNewerConcurrentMetadataReplyRemainsEligible(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	modelBinding(t, g, ui, native)
	for _, raw := range []string{
		`{"id":2,"method":"thread/read","params":{"threadId":"A","includeTurns":false}}`,
		`{"id":3,"method":"thread/read","params":{"threadId":"A","includeTurns":false}}`,
	} {
		write(t, ui, []byte(raw))
		_ = readWithin(t, native)
	}
	telemetryEvent(t, ui, native, `{"id":2,"result":{"thread":{"id":"A","status":{"type":"idle"},"model":"first","reasoningEffort":"low"}}}`)
	telemetryEvent(t, ui, native, `{"id":3,"result":{"thread":{"id":"A","status":{"type":"idle"},"model":"current","reasoningEffort":"high"}}}`)
	s := g.SessionState()
	t.Logf("model=%s effort=%s fresh=%v", *s.Model, *s.Effort, s.SettingsFresh)
	if *s.Model != "current" || *s.Effort != "high" {
		t.Fatal("an earlier read invalidated the newer read issued in the same settings revision")
	}
}
