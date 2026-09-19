package gateway

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func activityEvent(t *testing.T, ui, native *socketClient, thread, status string) {
	t.Helper()
	telemetryEvent(t, ui, native, fmt.Sprintf(`{"method":"thread/status/changed","params":{"threadId":%q,"status":%s}}`, thread, status))
}

func assertActivity(t *testing.T, g *Gateway, want string) {
	t.Helper()
	s := g.SessionState()
	if s.Activity == nil || *s.Activity != want || !s.ActivityFresh {
		t.Fatalf("activity=%v fresh=%v want=%s", s.Activity, s.ActivityFresh, want)
	}
}

func TestPrimaryActivityUsesStatusNotLastOutcome(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	modelBinding(t, g, ui, native)
	if g.SessionState().Activity != nil {
		t.Fatal("unknown initial status became idle")
	}
	for _, row := range []struct{ raw, want string }{
		{`{"type":"idle"}`, "idle"},
		{`{"type":"active","activeFlags":[]}`, "working"},
		{`{"type":"active","activeFlags":["waitingOnApproval","waitingOnUserInput"]}`, "working"},
		{`{"type":"systemError"}`, "system_error"},
		{`{"type":"notLoaded"}`, "not_loaded"},
	} {
		activityEvent(t, ui, native, "A", row.raw)
		assertActivity(t, g, row.want)
	}
	activityEvent(t, ui, native, "A", `{"type":"active","activeFlags":["waitingOnApproval","waitingOnUserInput"]}`)
	s := g.SessionState()
	if len(s.WaitingFor) != 2 || s.WaitingFor[0] != "approval" || s.WaitingFor[1] != "input" {
		t.Fatal("waiting flags lost")
	}
	for _, status := range []string{"failed", "interrupted"} {
		telemetryEvent(t, ui, native, fmt.Sprintf(`{"method":"turn/completed","params":{"threadId":"A","turn":{"id":"T","status":%q}}}`, status))
		activityEvent(t, ui, native, "A", `{"type":"idle"}`)
		assertActivity(t, g, "idle")
	}
	activityEvent(t, ui, native, "A", `{"type":"future-status"}`)
	if g.SessionState().Activity != nil || g.SessionState().ActivityFresh {
		t.Fatal("unknown status guessed idle")
	}
}

func TestActivityPreACKAndReadOrdering(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprint(reverse), func(t *testing.T) {
			g, ui, peers, _ := setup(t)
			native := <-peers
			defer func() { _ = native.conn.Close() }()
			write(t, ui, []byte(startA))
			_ = readWithin(t, native)
			activityEvent(t, ui, native, "A", `{"type":"active","activeFlags":[]}`)
			telemetryEvent(t, ui, native, `{"method":"thread/started","params":{"thread":{"id":"A","status":{"type":"idle"}}}}`)
			telemetryEvent(t, ui, native, `{"id":1,"result":{"thread":{"id":"A","canAcceptDirectInput":true,"status":{"type":"idle"}}}}`)
			assertActivity(t, g, "working")
			metadataRequest(t, ui, native, 2)
			activityEvent(t, ui, native, "A", `{"type":"idle"}`)
			metadataReply(t, ui, native, 2, "fixture", "low", "systemError")
			assertActivity(t, g, "idle")
			metadataRequest(t, ui, native, 3)
			metadataRequest(t, ui, native, 4)
			if reverse {
				metadataReply(t, ui, native, 4, "fixture", "low", "idle")
				metadataReply(t, ui, native, 3, "fixture", "low", "systemError")
			} else {
				metadataReply(t, ui, native, 3, "fixture", "low", "systemError")
				metadataReply(t, ui, native, 4, "fixture", "low", "idle")
			}
			assertActivity(t, g, "idle")
		})
	}
}

func TestActivityPreservesPrimaryDuringSideAndReconnect(t *testing.T) {
	g, ui, peers, path := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	modelBinding(t, g, ui, native)
	activityEvent(t, ui, native, "A", `{"type":"active","activeFlags":[]}`)
	forkPair(t, ui, native, 2, "A", "side")
	activityEvent(t, ui, native, "side", `{"type":"systemError"}`)
	activityEvent(t, ui, native, "A", `{"type":"idle"}`)
	exchange(t, ui, native, `{"id":3,"method":"thread/inject_items","params":{"threadId":"side","items":[]}}`, `{"id":3,"result":{}}`)
	assertActivity(t, g, "idle")
	activityEvent(t, ui, native, "side", `{"type":"active","activeFlags":["waitingOnApproval"]}`)
	assertActivity(t, g, "idle")
	telemetryEvent(t, ui, native, compactionEvent("side", "side-item", true))
	if len(g.SessionState().CompactionEvents) != 0 {
		t.Fatal("side produced primary compaction cue")
	}
	old := g.currentConnection()
	_ = ui.conn.Close()
	select {
	case <-old.cleaned:
	case <-time.After(time.Second):
		t.Fatal("close timeout")
	}
	if s := g.SessionState(); s.Fresh || s.ActivityFresh {
		t.Fatal("disconnected activity stayed fresh")
	}
	next, err := dialSocket(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = next.conn.Close() }()
	resumed := <-peers
	defer func() { _ = resumed.conn.Close() }()
	exchange(t, next, resumed, `{"id":0,"method":"initialize"}`, `{"id":0,"result":{}}`)
	exchange(t, next, resumed, `{"id":1,"method":"thread/resume","params":{"threadId":"A","config":{},"runtimeWorkspaceRoots":[]}}`, `{"id":1,"result":{"thread":{"id":"A","canAcceptDirectInput":true,"status":{"type":"active","activeFlags":[]}}}}`)
	assertActivity(t, g, "working")
	forkPair(t, next, resumed, 2, "A", "fork")
	activityEvent(t, next, resumed, "fork", `{"type":"active","activeFlags":["waitingOnUserInput"]}`)
	exchange(t, next, resumed, `{"id":3,"method":"thread/unsubscribe","params":{"threadId":"A"}}`, `{"id":3,"result":{"status":"unsubscribed"}}`)
	assertActivity(t, g, "working")
	if s := g.SessionState(); s.Thread != "fork" || len(s.WaitingFor) != 1 || s.WaitingFor[0] != "input" {
		t.Fatal("accepted fork lost its own activity")
	}
}

func TestCompactionCueIsCompletedOnlyAndDoesNotForceActivity(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	modelBinding(t, g, ui, native)
	activityEvent(t, ui, native, "A", `{"type":"active","activeFlags":[]}`)
	telemetryEvent(t, ui, native, compactionEvent("A", "failed", false))
	if s := g.SessionState(); len(s.CompactionEvents) != 0 || !*s.Compacting {
		t.Fatal("start created completed notice")
	}
	telemetryEvent(t, ui, native, `{"method":"turn/completed","params":{"threadId":"A","turn":{"id":"T","status":"interrupted"}}}`)
	if s := g.SessionState(); len(s.CompactionEvents) != 0 || *s.Compacting {
		t.Fatal("interruption created completion or kept progress")
	}
	for range 2 {
		telemetryEvent(t, ui, native, compactionEvent("A", "done", true))
	}
	s := g.SessionState()
	if len(s.CompactionEvents) != 1 || s.CompactionEvents[0].Sequence != 1 || s.CompactionEvents[0].ObservedAt.IsZero() {
		t.Fatal("completed cue missing or duplicated")
	}
	assertActivity(t, g, "working")
	s.CompactionEvents[0].Sequence = 999
	if g.SessionState().CompactionEvents[0].Sequence != 1 {
		t.Fatal("snapshot aliases mutable event tail")
	}
	activityEvent(t, ui, native, "A", `{"type":"idle"}`)
	assertActivity(t, g, "idle")
	exchange(t, ui, native, `{"id":2,"method":"thread/resume","params":{"threadId":"A","config":{},"runtimeWorkspaceRoots":[]}}`, `{"id":2,"result":{"thread":{"id":"A","canAcceptDirectInput":true,"status":{"type":"idle"},"turns":[{"items":[{"type":"contextCompaction","id":"historical"}]}]}}}`)
	telemetryEvent(t, ui, native, compactionEvent("A", "done", true))
	if len(g.SessionState().CompactionEvents) != 1 {
		t.Fatal("resume/history/replay created a new completed cue")
	}
}

func TestCompactionNotificationTailIsBounded(t *testing.T) {
	g := New(Config{Epoch: "bounded"})
	entry := &threadObservation{}
	for i := range maxCompactionEvents + 2 {
		g.telemetry.compaction(entry, compactionObservation{thread: "A", item: fmt.Sprint(i), turn: "T", completed: true, observedAt: time.Now()})
	}
	s := g.SessionState()
	if len(s.CompactionEvents) != maxCompactionEvents || s.CompactionEvents[0].Sequence != 3 || *s.Compactions != uint64(maxCompactionEvents+2) {
		t.Fatal("notification tail or cumulative count bound incorrect")
	}
}
