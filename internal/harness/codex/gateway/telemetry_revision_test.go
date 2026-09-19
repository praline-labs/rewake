package gateway

import (
	"fmt"
	"testing"
)

func TestSettingsRejectionsOnlyResolveTheirOwnProposal(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprint(reverse), func(t *testing.T) {
			g, ui, peers, _ := setup(t)
			native := <-peers
			defer func() { _ = native.conn.Close() }()
			modelBinding(t, g, ui, native)
			telemetryEvent(t, ui, native, usageEvent("A", "old", "121200", "272000"))
			for _, id := range []int{2, 3} {
				write(t, ui, []byte(fmt.Sprintf(`{"id":%d,"method":"thread/settings/update","params":{"threadId":"A","model":"proposed"}}`, id)))
				_ = readWithin(t, native)
			}
			first, last := 2, 3
			if reverse {
				first, last = 3, 2
			}
			telemetryEvent(t, ui, native, fmt.Sprintf(`{"id":%d,"error":{"code":-32602}}`, first))
			telemetryEvent(t, ui, native, `{"method":"turn/started","params":{"threadId":"A","turn":{"id":"new"}}}`)
			telemetryEvent(t, ui, native, usageEvent("A", "new", "122000", "272000"))
			s := g.SessionState()
			if s.SettingsFresh || s.ContextFresh || *s.Model != "first" {
				t.Fatal("rejection cleared a different pending proposal")
			}
			telemetryEvent(t, ui, native, fmt.Sprintf(`{"id":%d,"error":{"code":-32602}}`, last))
			s = g.SessionState()
			if !s.SettingsFresh || !s.ContextFresh || *s.Model != "first" || *s.Effort != "low" {
				t.Fatal("all rejected proposals did not restore confirmed settings/usage")
			}
		})
	}
}

func TestDelayedReadCannotClearNewProposalOrNewSelection(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	modelBinding(t, g, ui, native)
	write(t, ui, []byte(`{"id":2,"method":"thread/read","params":{"threadId":"A","includeTurns":false}}`))
	_ = readWithin(t, native)
	exchange(t, ui, native, `{"id":3,"method":"thread/settings/update","params":{"threadId":"A","model":"new"}}`, `{"id":3,"result":{}}`)
	telemetryEvent(t, ui, native, `{"id":2,"result":{"thread":{"id":"A","status":{"type":"idle"},"model":"old-read","reasoningEffort":"low"}}}`)
	if s := g.SessionState(); s.SettingsFresh || *s.Model != "first" {
		t.Fatal("old read cleared a newer pending proposal")
	}
	write(t, ui, []byte(`{"id":4,"method":"thread/read","params":{"threadId":"A","includeTurns":false}}`))
	_ = readWithin(t, native)
	exchange(t, ui, native, `{"id":5,"method":"thread/resume","params":{"threadId":"A","config":{},"runtimeWorkspaceRoots":[]}}`, `{"id":5,"result":{"thread":{"id":"A","canAcceptDirectInput":true},"model":"selected","reasoningEffort":"high"}}`)
	telemetryEvent(t, ui, native, `{"id":4,"result":{"thread":{"id":"A","status":{"type":"idle"},"model":"old-read","reasoningEffort":"low"}}}`)
	s := g.SessionState()
	if *s.Model != "selected" || *s.Effort != "high" || !s.SettingsFresh {
		t.Fatal("old-generation read crossed the selection fence")
	}
}

func TestOldTerminalDoesNotClearNewCompactionAndStagedFailureRetires(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	write(t, ui, []byte(startA))
	_ = readWithin(t, native)
	telemetryEvent(t, ui, native, compactionEvent("A", "pre-ack", false))
	telemetryEvent(t, ui, native, `{"method":"turn/completed","params":{"threadId":"A","turn":{"id":"T","status":"failed"}}}`)
	telemetryEvent(t, ui, native, `{"id":1,"result":{"thread":{"id":"A","canAcceptDirectInput":true},"model":"first"}}`)
	if s := assertCompactions(t, g, 0); *s.Compacting {
		t.Fatal("failed pre-ACK compaction remained active")
	}
	telemetryEvent(t, ui, native, compactionEvent("A", "old", false))
	telemetryEvent(t, ui, native, `{"method":"item/started","params":{"threadId":"A","turnId":"new-turn","item":{"type":"contextCompaction","id":"new"}}}`)
	telemetryEvent(t, ui, native, `{"method":"turn/completed","params":{"threadId":"A","turn":{"id":"T","status":"interrupted"}}}`)
	if s := assertCompactions(t, g, 0); !*s.Compacting {
		t.Fatal("old turn cleared another turn's compaction")
	}
	telemetryEvent(t, ui, native, `{"method":"turn/completed","params":{"threadId":"A","turn":{"id":"new-turn","status":"interrupted"}}}`)
	if s := assertCompactions(t, g, 0); *s.Compacting {
		t.Fatal("own interrupted turn did not clear progress")
	}
}

func TestAcceptedPrimaryCacheRecoveryKeepsCompactionDedup(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	modelBinding(t, g, ui, native)
	telemetryEvent(t, ui, native, compactionEvent("A", "completed", true))
	write(t, ui, []byte(`{"id":2,"method":"thread/start","params":{"threadSource":"user","runtimeWorkspaceRoots":[]}}`))
	_ = readWithin(t, native)
	for i := range maxObservationThreads {
		telemetryEvent(t, ui, native, usageEvent(fmt.Sprintf("other-%d", i), "T", "12000", "272000"))
	}
	telemetryEvent(t, ui, native, `{"id":2,"result":{"thread":{"id":"B","canAcceptDirectInput":true},"model":"second"}}`)
	if s := assertCompactions(t, g, 1); !s.Fresh || s.Coverage != "partial" || *s.Model != "second" {
		t.Fatal("cache recovery lost coverage or count")
	}
	exchange(t, ui, native, `{"id":3,"method":"thread/resume","params":{"threadId":"A","config":{},"runtimeWorkspaceRoots":[]}}`, `{"id":3,"result":{"thread":{"id":"A","canAcceptDirectInput":true},"model":"first"}}`)
	telemetryEvent(t, ui, native, compactionEvent("A", "completed", true))
	assertCompactions(t, g, 1)
}

func TestSettingsRejectionSurvivesPrimaryPreservingSide(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	modelBinding(t, g, ui, native)
	write(t, ui, []byte(`{"id":2,"method":"thread/settings/update","params":{"threadId":"A","model":"invalid"}}`))
	_ = readWithin(t, native)
	forkPair(t, ui, native, 3, "A", "side")
	exchange(t, ui, native, `{"id":4,"method":"thread/inject_items","params":{"threadId":"side","items":[]}}`, `{"id":4,"result":{}}`)
	telemetryEvent(t, ui, native, `{"id":2,"error":{"code":-32602}}`)
	telemetryEvent(t, ui, native, `{"method":"turn/started","params":{"threadId":"A","turn":{"id":"new"}}}`)
	telemetryEvent(t, ui, native, usageEvent("A", "new", "122000", "272000"))
	s := g.SessionState()
	if s.Thread != "A" || !s.SettingsFresh || !s.ContextFresh {
		t.Fatal("side generation change stranded the preserved parent's proposal")
	}
}
