package gateway

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func callbacks(g *Gateway) chan Completion {
	out := make(chan Completion, 16)
	g.mu.Lock()
	g.cfg.Complete = func(v Completion) { out <- v }
	g.mu.Unlock()
	return out
}

func unknownRead(t *testing.T, ui, server *socketClient) {
	t.Helper()
	exchange(t, ui, server, `{"id":50,"method":"thread/read","params":{"threadId":"B"}}`, `{"id":50,"result":{"thread":{"id":"B"}}}`)
}

func TestNativeACKAfterBindingChangePreservesKnownScope(t *testing.T) {
	g, ui, peers, _ := setup(t)
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	out := callbacks(g)
	bindUI(t, g, ui, server)
	ticket := g.Binding()
	write(t, ui, []byte(`{"id":7,"method":"turn/start","params":{"threadId":"A","input":[]}}`))
	_ = readWithin(t, server)
	unknownRead(t, ui, server)
	for _, raw := range []string{`{"method":"turn/started","params":{"threadId":"A","turn":{"id":"T"}}}`, `{"method":"item/completed","params":{"threadId":"A","turnId":"T","item":{"type":"agentMessage","text":"final fixture"}}}`, `{"method":"turn/completed","params":{"threadId":"A","turn":{"id":"T","status":"completed"}}}`} {
		write(t, server, []byte(raw))
		_ = readWithin(t, ui)
	}
	select {
	case v := <-out:
		t.Fatal("pending work published without ack", v)
	default:
	}
	write(t, server, []byte(`{"id":7,"result":{"turn":{"id":"T"}}}`))
	_ = readWithin(t, ui)
	select {
	case v := <-out:
		if v.Text != "final fixture" || v.Generation != ticket.Generation || v.ID != "A/T" {
			t.Fatal(v)
		}
	default:
		t.Fatal("late ack lost already observed result")
	}
	if g.Binding().Ready {
		t.Fatal("ack/result promoted old routing")
	}
}

func TestServerRequestIDCannotActAsWorkAcknowledgement(t *testing.T) {
	g, ui, peers, _ := setup(t)
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	out := callbacks(g)
	bindUI(t, g, ui, server)
	write(t, ui, []byte(`{"id":7,"method":"turn/steer","params":{"threadId":"A"}}`))
	_ = readWithin(t, server)
	unknownRead(t, ui, server)
	write(t, server, []byte(`{"id":7,"method":"item/tool/requestUserInput","params":{"threadId":"A","turnId":"T"}}`))
	_ = readWithin(t, ui)
	write(t, server, []byte(`{"method":"turn/completed","params":{"threadId":"A","turn":{"id":"T","status":"completed"}}}`))
	_ = readWithin(t, ui)
	write(t, server, []byte(`{"id":7,"error":{"code":-32600}}`))
	_ = readWithin(t, ui)
	select {
	case v := <-out:
		t.Fatal("server request or refused ack fabricated admission", v)
	default:
	}
	c := g.currentConnection()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.admitted.pending) != 0 || len(c.admitted.threads) != 0 {
		t.Fatal("refused work retained state")
	}
}

func TestConfirmedOutcomeSurvivesNewPrimaryAndSteerResponseShape(t *testing.T) {
	g, ui, peers, _ := setup(t)
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	out := callbacks(g)
	bindUI(t, g, ui, server)
	exchange(t, ui, server, `{"id":7,"method":"turn/steer","params":{"threadId":"A"}}`, `{"id":7,"result":{"turnId":"T"}}`)
	exchange(t, ui, server, `{"id":8,"method":"thread/resume","params":{"threadId":"B","config":{},"runtimeWorkspaceRoots":[]}}`, `{"id":8,"result":{"thread":{"id":"B","canAcceptDirectInput":true}}}`)
	write(t, server, []byte(`{"method":"turn/completed","params":{"threadId":"child","turn":{"id":"T","status":"completed"}}}`))
	_ = readWithin(t, ui)
	write(t, server, []byte(`{"method":"turn/completed","params":{"threadId":"A","turn":{"id":"T","status":"failed","error":{"message":"fixture failure"}}}}`))
	_ = readWithin(t, ui)
	select {
	case v := <-out:
		if v.ID != "A/T" || v.Kind != "error" || v.Text != "fixture failure" {
			t.Fatal(v)
		}
	default:
		t.Fatal("new primary erased admitted result")
	}
	if b := g.Binding(); !b.Ready || b.Thread != "B" {
		t.Fatal("completion restored A", b)
	}
	select {
	case v := <-out:
		t.Fatal("nested duplicate", v)
	default:
	}
}

func TestConnectionEndClearsAdmissionWithoutInventingResult(t *testing.T) {
	g, ui, peers, _ := setup(t)
	server := <-peers
	out := callbacks(g)
	bindUI(t, g, ui, server)
	done := make(chan error, 1)
	go func() { _, e := g.Deliver(context.Background(), g.Binding(), "id", "notice"); done <- e }()
	m := metadata(t, string(readWithin(t, server)))
	write(t, server, []byte(fmt.Sprintf(`{"id":%q,"result":{"turn":{"id":"T"}}}`, m.idText)))
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	c := g.currentConnection()
	_ = server.conn.Close()
	<-c.cleaned
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.admitted.pending) != 0 || len(c.admitted.threads) != 0 {
		t.Fatal("closed connection retained admitted scopes")
	}
	select {
	case v := <-out:
		t.Fatal("disconnect fabricated result", v)
	default:
	}
}

func TestSteeredExistingIntervalKeepsGapEvidenceAfterRoutingLoss(t *testing.T) {
	g, ui, peers, _ := setup(t)
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	out := callbacks(g)
	bindUI(t, g, ui, server)
	write(t, server, []byte(`{"method":"turn/started","params":{"threadId":"A","turn":{"id":"T"}}}`))
	_ = readWithin(t, ui)
	exchange(t, ui, server, `{"id":7,"method":"turn/steer","params":{"threadId":"A"}}`, `{"id":7,"result":{"turnId":"T"}}`)
	unknownRead(t, ui, server)
	write(t, server, []byte(`{"method":"thread/status/changed","params":{"threadId":"A","status":{"type":"idle"}}}`))
	_ = readWithin(t, ui)
	c := g.currentConnection()
	c.mu.Lock()
	c.admitted.expire(time.Now().Add(time.Second))
	results := c.collectOutcomes()
	c.mu.Unlock()
	c.complete(results)
	select {
	case v := <-out:
		if v.ID != "A/T" || v.Kind != "error" || v.Text != "completion not observed" {
			t.Fatal(v)
		}
	default:
		t.Fatal("pre-steer observed interval was erased")
	}
}
