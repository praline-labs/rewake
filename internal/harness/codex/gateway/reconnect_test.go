package gateway

import (
	"context"
	"testing"
	"time"
)

func awaitReleased(t *testing.T, g *Gateway) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		g.mu.Lock()
		count := len(g.conns)
		g.mu.Unlock()
		if count == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("closed connection retained")
}

func TestNativePreserveReconnectUsesRequestedPriorScopeAndNewACK(t *testing.T) {
	g, ui, peers, path := setup(t)
	native := <-peers
	bindUI(t, g, ui, native)
	old := g.Binding()
	_ = ui.conn.Close()
	_ = native.conn.Close()
	awaitReleased(t, g)
	// Failed reconnect attempts keep only an ID anchor, never closed clients (R14-1).
	for i := 0; i < 5; i++ {
		next, err := dialSocket(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		peer := <-peers
		exchange(t, next, peer, `{"id":0,"method":"initialize"}`, `{"id":0,"result":{}}`)
		exchange(t, next, peer, `{"id":1,"method":"thread/resume","params":{"threadId":"A","excludeTurns":true}}`, `{"id":1,"error":{"code":-32600,"message":"temporarily unavailable"}}`)
		if g.Binding().Ready {
			t.Fatal("refused reconnect became ready")
		}
		_ = next.conn.Close()
		_ = peer.conn.Close()
		awaitReleased(t, g)
	}
	next, err := dialSocket(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = next.conn.Close() }()
	peer := <-peers
	defer func() { _ = peer.conn.Close() }()
	exchange(t, next, peer, `{"id":0,"method":"initialize"}`, `{"id":0,"result":{}}`)
	exchange(t, next, peer, `{"id":1,"method":"thread/resume","params":{"threadId":"A","excludeTurns":true}}`, `{"id":1,"result":{"thread":{"id":"A","canAcceptDirectInput":true}}}`)
	current := g.Binding()
	if !current.Ready || current.Thread != "A" || current.Connection == old.Connection {
		t.Fatal(current)
	}
	if _, err := g.Deliver(context.Background(), old, "old", "notice"); err == nil {
		t.Fatal("old connection admitted replay")
	}
	// PreserveExistingThread replays the selected cached view; it does not run
	// ordinary resume backfill or require a fabricated goal/get to unblock work.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	reserved, err := g.Reserve(ctx)
	if err != nil {
		t.Fatal("preserve reconnect never became deliverable", err)
	}
	reserved.Close()
}

func TestReconnectNeverPromotesUnknownTargetOrUncertainOldScope(t *testing.T) {
	for _, variant := range []string{"no predecessor", "different target", "uncertain predecessor"} {
		t.Run(variant, func(t *testing.T) {
			g, ui, peers, path := setup(t)
			native := <-peers
			if variant != "no predecessor" {
				bindUI(t, g, ui, native)
			}
			if variant == "uncertain predecessor" {
				exchange(t, ui, native, `{"id":2,"method":"thread/read","params":{"threadId":"other"}}`, `{"id":2,"result":{"thread":{"id":"other"}}}`)
			}
			_ = ui.conn.Close()
			_ = native.conn.Close()
			awaitReleased(t, g)
			next, err := dialSocket(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = next.conn.Close() }()
			peer := <-peers
			defer func() { _ = peer.conn.Close() }()
			target := "A"
			if variant == "different target" {
				target = "B"
			}
			exchange(t, next, peer, `{"id":0,"method":"initialize"}`, `{"id":0,"result":{}}`)
			exchange(t, next, peer, `{"id":1,"method":"thread/resume","params":{"threadId":"`+target+`","excludeTurns":true}}`, `{"id":1,"result":{"thread":{"id":"`+target+`","canAcceptDirectInput":true}}}`)
			if g.Binding().Ready {
				t.Fatal("unproved reconnect target promoted")
			}
		})
	}
}

func TestPendingAcceptedIntentKeepsObservedActiveIdleGap(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	completed := make(chan Completion, 2)
	g.cfg.Complete = func(c Completion) { completed <- c }
	write(t, ui, []byte(startA))
	_ = readWithin(t, native)
	for _, raw := range []string{
		`{"method":"thread/status/changed","params":{"threadId":"A","status":{"type":"active"}}}`,
		`{"method":"thread/status/changed","params":{"threadId":"A","status":{"type":"idle"}}}`,
	} {
		write(t, native, []byte(raw))
		_ = readWithin(t, ui)
	}
	write(t, native, []byte(`{"id":1,"result":{"thread":{"id":"A","canAcceptDirectInput":true,"status":{"type":"idle"}}}}`))
	_ = readWithin(t, ui)
	select {
	case result := <-completed:
		if result.Kind != "stopped" || result.Thread != "A" || result.Text != gapText {
			t.Fatal(result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("R14-2: accepted-intent discovery lost observed interval")
	}
}

func TestCompetingReconnectRetiresTheOldCorrelationAnchor(t *testing.T) {
	g, ui, peers, path := setup(t)
	native := <-peers
	bindUI(t, g, ui, native)
	_ = ui.conn.Close()
	_ = native.conn.Close()
	awaitReleased(t, g)
	first, err := dialSocket(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	a := <-peers
	exchange(t, first, a, `{"id":0,"method":"initialize"}`, `{"id":0,"result":{}}`)
	write(t, first, []byte(`{"id":1,"method":"thread/resume","params":{"threadId":"A"}}`))
	_ = readWithin(t, a)
	second, err := dialSocket(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	b := <-peers
	write(t, second, []byte(startA))
	_ = readWithin(t, b)
	g.mu.Lock()
	anchor := g.reconnectThread
	g.mu.Unlock()
	if anchor != "" {
		t.Fatal("conflict preserved an old reconnect authority")
	}
	_ = first.conn.Close()
	_ = second.conn.Close()
	_ = a.conn.Close()
	_ = b.conn.Close()
	awaitReleased(t, g)
	next, err := dialSocket(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = next.conn.Close() }()
	peer := <-peers
	defer func() { _ = peer.conn.Close() }()
	exchange(t, next, peer, `{"id":0,"method":"initialize"}`, `{"id":0,"result":{}}`)
	exchange(t, next, peer, `{"id":1,"method":"thread/resume","params":{"threadId":"A"}}`, `{"id":1,"result":{"thread":{"id":"A","canAcceptDirectInput":true}}}`)
	if g.Binding().Ready {
		t.Fatal("closed conflict promoted an old target")
	}
}
