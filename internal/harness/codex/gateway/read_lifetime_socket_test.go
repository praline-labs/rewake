package gateway

import (
	"context"
	"fmt"
	"testing"
)

func exchange(t *testing.T, ui, server *socketClient, request, response string) {
	t.Helper()
	write(t, ui, []byte(request))
	_ = readWithin(t, server)
	write(t, server, []byte(response))
	_ = readWithin(t, ui)
}

func socketResume(t *testing.T, ui, server *socketClient) {
	t.Helper()
	exchange(t, ui, server, `{"id":2,"method":"thread/resume","params":{"threadId":"A","config":{},"runtimeWorkspaceRoots":[]}}`, `{"id":2,"result":{"thread":{"id":"A","canAcceptDirectInput":true}}}`)
}

func TestInjectedWorkRetiresUnopenedPendingAndPartialContexts(t *testing.T) {
	for _, method := range []string{"turn/start", "turn/steer"} {
		for _, phase := range []string{"armed", "pending", "partial"} {
			t.Run(method+"/"+phase, func(t *testing.T) {
				g, ui, peers, _ := setup(t)
				server := <-peers
				defer func() { _ = server.conn.Close() }()
				bindUI(t, g, ui, server)
				socketResume(t, ui, server)
				ticket := g.Binding()
				if phase != "armed" {
					write(t, ui, []byte(`{"id":3,"method":"thread/loaded/list","params":{}}`))
					_ = readWithin(t, server)
				}
				if phase == "partial" {
					write(t, server, []byte(`{"id":3,"result":{"data":["A","B","C"]}}`))
					_ = readWithin(t, ui)
					exchange(t, ui, server, `{"id":4,"method":"thread/read","params":{"threadId":"B"}}`, `{"id":4,"result":{"thread":{"id":"B"}}}`)
				}
				done := make(chan error, 1)
				go func() { _, err := g.callBound(context.Background(), ticket, method, map[string]any{}); done <- err }()
				sent := metadata(t, string(readWithin(t, server)))
				if phase == "pending" {
					write(t, server, []byte(`{"id":3,"result":{"data":["A","B","C"]}}`))
					_ = readWithin(t, ui)
				}
				write(t, server, []byte(fmt.Sprintf(`{"id":%q,"result":{"turn":{"id":"task"}}}`, sent.idText)))
				if err := <-done; err != nil {
					t.Fatal(err)
				}
				if g.Binding() != ticket {
					t.Fatal("revoking reads changed the accepted target")
				}
				if phase == "armed" {
					exchange(t, ui, server, `{"id":3,"method":"thread/loaded/list","params":{}}`, `{"id":3,"result":{"data":["A","B","C"]}}`)
				}
				exchange(t, ui, server, `{"id":5,"method":"thread/read","params":{"threadId":"C"}}`, `{"id":5,"result":{"thread":{"id":"C"}}}`)
				if g.Binding().Ready {
					t.Fatal("injected work left stale read authorization")
				}
				if _, err := g.Deliver(context.Background(), ticket, "later", "notice"); err == nil {
					t.Fatal("stale target admitted work")
				}
				write(t, ui, []byte(`{"id":6,"method":"thread/list"}`))
				if m := metadata(t, string(readWithin(t, server))); m.method != "thread/list" {
					t.Fatal("refused work leaked onto wire")
				}
			})
		}
	}
}

func TestMaterializationPreservesReadWorkflow(t *testing.T) {
	g, ui, peers, _ := setup(t)
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	bindUI(t, g, ui, server)
	socketResume(t, ui, server)
	done := make(chan error, 1)
	go func() { done <- g.Materialize(context.Background(), g.Binding()) }()
	m := metadata(t, string(readWithin(t, server)))
	write(t, server, []byte(fmt.Sprintf(`{"id":%q,"result":{}}`, m.idText)))
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	exchange(t, ui, server, `{"id":3,"method":"thread/loaded/list","params":{}}`, `{"id":3,"result":{"data":["A","B"]}}`)
	exchange(t, ui, server, `{"id":4,"method":"thread/read","params":{"threadId":"B"}}`, `{"id":4,"result":{"thread":{"id":"B"}}}`)
	if !g.Binding().Ready {
		t.Fatal("name metadata revoked workflow")
	}
}

func TestAuxiliaryWorkForPrimaryClosesItsPendingList(t *testing.T) {
	g, ui, peers, path := setup(t)
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	bindUI(t, g, ui, server)
	socketResume(t, ui, server)
	write(t, ui, []byte(`{"id":3,"method":"thread/loaded/list"}`))
	_ = readWithin(t, server)
	aux, err := dialSocket(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = aux.conn.Close() }()
	peer := <-peers
	defer func() { _ = peer.conn.Close() }()
	exchange(t, aux, peer, `{"id":1,"method":"turn/steer","params":{"threadId":"A"}}`, `{"id":1,"error":{"code":-32600}}`)
	write(t, server, []byte(`{"id":3,"result":{"data":["A","B"]}}`))
	_ = readWithin(t, ui)
	exchange(t, ui, server, `{"id":4,"method":"thread/read","params":{"threadId":"B"}}`, `{"id":4,"result":{"thread":{"id":"B"}}}`)
	if g.Binding().Ready {
		t.Fatal("auxiliary work left pending grant live")
	}
}
