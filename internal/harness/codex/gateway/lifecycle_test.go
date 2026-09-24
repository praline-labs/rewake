package gateway

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestLateInjectedReplyCannotLeakAndOutcomeIsNotReplayed(t *testing.T) {
	g, ui, peers, _ := setup(t)
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	bindUI(t, g, ui, server)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	finished := make(chan error, 1)
	go func() { _, err := g.Deliver(ctx, g.Binding(), "same-message", "fixture"); finished <- err }()
	_ = readWithin(t, server)
	if err := <-finished; err == nil {
		t.Fatal("missing acknowledgement succeeded")
	}
	_ = server.conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := server.readMessage(); err == nil {
		t.Fatal("uncertain work was replayed")
	}
	if g.Binding().Ready {
		t.Fatal("timed-out admission kept readiness")
	}
}

func TestDuplicateInjectedResponseNeverReachesTUI(t *testing.T) {
	g, ui, peers, _ := setup(t)
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	bindUI(t, g, ui, server)
	done := make(chan error, 1)
	go func() { _, e := g.Deliver(context.Background(), g.Binding(), "id", "notice"); done <- e }()
	m := metadata(t, string(readWithin(t, server)))
	reply := []byte(fmt.Sprintf(`{"id":%q,"result":{"turn":{"id":"turn"}}}`, m.idText))
	write(t, server, reply)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	write(t, server, reply)
	marker := []byte(`{"method":"thread/status/changed","params":{"threadId":"A","status":{"type":"idle"}}}`)
	write(t, server, marker)
	if string(readWithin(t, ui)) != string(marker) {
		t.Fatal("duplicate internal response leaked")
	}
}

func TestClosedConnectionsReleaseStateWithoutObserverLeases(t *testing.T) {
	g, ui, peers, path := setup(t)
	server := <-peers
	bindUI(t, g, ui, server)
	old := g.currentConnection()
	next, e := dialSocket(context.Background(), path)
	if e != nil {
		t.Fatal(e)
	}
	other := <-peers
	write(t, next, []byte(`{"method":"initialized"}`))
	_ = readWithin(t, other)
	_ = ui.conn.Close()
	_ = server.conn.Close()
	select {
	case <-old.cleaned:
	case <-time.After(time.Second):
		t.Fatal("closed primary retained")
	}
	old.mu.Lock()
	size := len(old.state.pending) + len(old.injected) + len(old.state.events.watches)
	old.mu.Unlock()
	if size != 0 {
		t.Fatal("closed client retained state", size)
	}
	if g.Binding().Ready {
		t.Fatal("helper promoted after primary close")
	}
	bindUI(t, g, next, other)
	current := g.currentConnection()
	if current == old {
		t.Fatal("old incarnation reused")
	}
	_ = next.conn.Close()
	_ = other.conn.Close()
	select {
	case <-current.cleaned:
	case <-time.After(time.Second):
		t.Fatal("closed TUI retained backend")
	}
}

func TestAutomaticCompactionStillReportsRealTask(t *testing.T) {
	o := newObserver()
	now := time.Now()
	o.bind("A", "idle", now)
	o.event(meta{method: "turn/started", thread: "A", turn: "task"}, nil, now)
	o.event(meta{method: "item/completed", thread: "A", turn: "task"}, []byte(`{"params":{"item":{"type":"contextCompaction"}}}`), now)
	o.event(meta{method: "turn/completed", thread: "A", turn: "task", status: "completed"}, nil, now)
	if out := o.drain(); len(out) != 1 || out[0].Kind != "finished" {
		t.Fatal("automatic compaction hid real task", out)
	}
}

func TestActiveResponseSnapshotAfterTerminalEventKeepsGap(t *testing.T) {
	for _, status := range []string{"idle", "systemError"} {
		now := time.Now()
		o := newObserver()
		o.event(meta{method: "thread/status/changed", thread: "A", status: status}, nil, now)
		o.bind("A", "active", now)
		o.expire(now.Add(time.Second))
		if out := o.drain(); len(out) != 1 || out[0].Kind != "stopped" || out[0].Text != gapText {
			t.Fatal(status, out)
		}
		o.expire(now.Add(2 * time.Second))
		if len(o.drain()) != 0 {
			t.Fatal("duplicate gap")
		}
	}
}
