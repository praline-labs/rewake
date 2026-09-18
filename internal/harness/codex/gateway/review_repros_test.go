package gateway

import (
	"testing"
	"time"
)

func TestReviewManualCompactionRefusalDoesNotReopenFinishedTurn(t *testing.T) {
	g, ui, peers, _ := setup(t)
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	bindUI(t, g, ui, server)
	for _, raw := range []string{
		`{"method":"turn/started","params":{"threadId":"A","turn":{"id":"done"}}}`,
		`{"method":"thread/status/changed","params":{"threadId":"A","status":{"type":"idle"}}}`,
		`{"method":"turn/completed","params":{"threadId":"A","turn":{"id":"done","status":"completed"}}}`,
	} {
		write(t, server, []byte(raw))
		_ = readWithin(t, ui)
	}
	write(t, ui, []byte(`{"id":2,"method":"thread/compact/start","params":{"threadId":"A"}}`))
	_ = readWithin(t, server)
	write(t, server, []byte(`{"id":2,"error":{"code":-32600,"message":"compaction refused"}}`))
	_ = readWithin(t, ui)
	c := g.currentConnection()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state.events.expire(time.Now().Add(time.Second))
	if out := c.state.events.drain(); len(out) != 0 {
		t.Fatalf("refused maintenance reopened a finished task: %+v", out)
	}
}

func TestReviewEarlierGapSurvivesFollowingTurn(t *testing.T) {
	now := time.Now()
	o := newObserver()
	o.bind("A", "idle", now)
	o.event(meta{method: "turn/started", thread: "A", turn: "first"}, nil, now)
	o.event(meta{method: "thread/status/changed", thread: "A", status: "idle"}, nil, now)
	o.event(meta{method: "turn/started", thread: "A", turn: "second"}, nil, now.Add(100*time.Millisecond))
	o.event(meta{method: "turn/completed", thread: "A", turn: "second", status: "completed"}, nil, now.Add(200*time.Millisecond))
	o.expire(now.Add(time.Second))
	out := o.drain()
	if len(out) != 2 {
		t.Fatalf("earlier missing completion disappeared behind following turn: %+v", out)
	}
}
