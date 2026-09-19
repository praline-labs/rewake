package gateway

import (
	"fmt"
	"testing"
)

func TestReviewObsoleteLifecycleReplyCannotSeedNewSelection(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	for _, raw := range []string{
		`{"id":1,"method":"thread/resume","params":{"threadId":"A","config":{},"runtimeWorkspaceRoots":[]}}`,
		`{"id":2,"method":"thread/resume","params":{"threadId":"A","config":{},"runtimeWorkspaceRoots":[]}}`,
	} {
		write(t, ui, []byte(raw))
		_ = readWithin(t, native)
	}
	telemetryEvent(t, ui, native, `{"id":1,"result":{"thread":{"id":"A","canAcceptDirectInput":true},"model":"obsolete","reasoningEffort":"low"}}`)
	telemetryEvent(t, ui, native, `{"id":2,"result":{"thread":{"id":"A","canAcceptDirectInput":true},"model":"current","reasoningEffort":"high"}}`)
	s := g.SessionState()
	if !g.Binding().Ready || s.Model == nil || s.Effort == nil {
		t.Fatal("invalid reproduction state", s)
	}
	t.Logf("selected generation=%d model=%s effort=%s fresh=%v", g.Binding().Generation, *s.Model, *s.Effort, s.SettingsFresh)
	if *s.Model != "current" || *s.Effort != "high" {
		t.Fatal("obsolete selection reply was promoted to fresh current settings")
	}
}

func TestReviewDelayedMetadataCannotUndoAppliedSettings(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	modelBinding(t, g, ui, native)
	write(t, ui, []byte(`{"id":2,"method":"thread/read","params":{"threadId":"A","includeTurns":false}}`))
	_ = readWithin(t, native)
	telemetryEvent(t, ui, native, `{"method":"thread/settings/updated","params":{"threadId":"A","threadSettings":{"model":"current","effort":"high"}}}`)
	telemetryEvent(t, ui, native, `{"id":2,"result":{"thread":{"id":"A","status":{"type":"idle"},"model":"obsolete","reasoningEffort":"low"}}}`)
	s := g.SessionState()
	if s.Model == nil || s.Effort == nil {
		t.Fatal("settings disappeared", s)
	}
	t.Logf("model=%s effort=%s fresh=%v", *s.Model, *s.Effort, s.SettingsFresh)
	if *s.Model != "current" || *s.Effort != "high" {
		t.Fatal("old metadata reply undid newer applied settings")
	}
}

func TestReviewFailedCompactionDoesNotRemainInProgress(t *testing.T) {
	for _, status := range []string{"failed", "interrupted"} {
		t.Run(status, func(t *testing.T) {
			g, ui, peers, _ := setup(t)
			native := <-peers
			defer func() { _ = native.conn.Close() }()
			modelBinding(t, g, ui, native)
			telemetryEvent(t, ui, native, compactionEvent("A", "unfinished", false))
			telemetryEvent(t, ui, native, `{"method":"turn/completed","params":{"threadId":"A","turn":{"id":"T","status":"`+status+`"}}}`)
			s := g.SessionState()
			t.Logf("fresh=%v compacting=%v count=%d", s.Fresh, *s.Compacting, *s.Compactions)
			if s.Compacting == nil || *s.Compacting || s.Compactions == nil || *s.Compactions != 0 {
				t.Fatal("ended compaction still advertised as in progress")
			}
		})
	}
}

func TestReviewRejectedSettingsDoesNotRemainPending(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	modelBinding(t, g, ui, native)
	telemetryEvent(t, ui, native, usageEvent("A", "old", "121200", "272000"))
	exchange(t, ui, native, `{"id":2,"method":"thread/settings/update","params":{"threadId":"A","model":"invalid"}}`, `{"id":2,"error":{"code":-32602,"message":"unknown model"}}`)
	telemetryEvent(t, ui, native, `{"method":"turn/started","params":{"threadId":"A","turn":{"id":"new"}}}`)
	telemetryEvent(t, ui, native, usageEvent("A", "new", "122000", "272000"))
	s := g.SessionState()
	t.Logf("model=%s settingsFresh=%v contextFresh=%v used=%d", *s.Model, s.SettingsFresh, s.ContextFresh, *s.ContextUsed)
	if !s.SettingsFresh || !s.ContextFresh {
		t.Fatal("rejected proposal kept old confirmed settings and subsequent usage permanently pending")
	}
}

func TestReviewAcceptedPrimaryRecoversFromFullCandidateCache(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	write(t, ui, []byte(startA))
	_ = readWithin(t, native)
	for i := range maxObservationThreads {
		telemetryEvent(t, ui, native, usageEvent(fmt.Sprintf("candidate-%d", i), "T", "12000", "272000"))
	}
	telemetryEvent(t, ui, native, `{"id":1,"result":{"thread":{"id":"A","canAcceptDirectInput":true},"model":"current","reasoningEffort":"high"}}`)
	telemetryEvent(t, ui, native, usageEvent("A", "T", "121200", "272000"))
	s := g.SessionState()
	t.Logf("binding=%+v snapshot=%+v", g.Binding(), s)
	if !g.Binding().Ready || s.Model == nil || *s.Model != "current" || s.ContextUsed == nil || *s.ContextUsed != 121200 {
		t.Fatal("accepted primary could not displace obsolete pre-ACK candidates")
	}
}
