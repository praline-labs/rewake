package gateway

import (
	"context"
	"fmt"
	"testing"
)

// Same overlap sequence as O4-1; require its already admitted outcome to survive.
func TestAdmittedOverlapCompletionSurvivesUncertainRouting(t *testing.T) {
	g, ui, peers, _ := setup(t)
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	results := make(chan Completion, 4)
	g.mu.Lock()
	g.cfg.Complete = func(c Completion) { results <- c }
	g.mu.Unlock()
	bindUI(t, g, ui, server)
	socketResume(t, ui, server)
	ticket := g.Binding()
	exchange(t, ui, server, `{"id":3,"method":"thread/loaded/list"}`, `{"id":3,"result":{"data":["A","B"]}}`)
	done := make(chan error, 1)
	go func() { _, err := g.Deliver(context.Background(), ticket, "overlap", "fixture"); done <- err }()
	sent := metadata(t, string(readWithin(t, server)))
	if sent.thread != "A" {
		t.Fatalf("wrong injected target: %s", sent.thread)
	}
	write(t, server, []byte(fmt.Sprintf(`{"id":%q,"result":{"turn":{"id":"task"}}}`, sent.idText)))
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`{"method":"thread/status/changed","params":{"threadId":"A","status":{"type":"active"}}}`,
		`{"method":"turn/started","params":{"threadId":"A","turn":{"id":"task"}}}`,
	} {
		write(t, server, []byte(raw))
		_ = readWithin(t, ui)
	}
	// Native scan continues after injected admission retired its read permission.
	exchange(t, ui, server, `{"id":4,"method":"thread/read","params":{"threadId":"B"}}`, `{"id":4,"result":{"thread":{"id":"B"}}}`)
	if g.Binding().Ready {
		t.Fatal("overlap did not fail unavailable as declared")
	}
	for _, raw := range []string{
		`{"method":"thread/status/changed","params":{"threadId":"A","status":{"type":"idle"}}}`,
		`{"method":"turn/completed","params":{"threadId":"A","turn":{"id":"task","status":"completed"}}}`,
	} {
		write(t, server, []byte(raw))
		_ = readWithin(t, ui)
	}
	select {
	case result := <-results:
		if result.Thread != "A" || result.ID != "A/task" || result.Kind != "finished" || result.Epoch != ticket.Epoch || result.Connection != ticket.Connection {
			t.Fatalf("wrong retained outcome: %+v", result)
		}
	default:
		t.Fatal("admitted completion was lost after routing became uncertain")
	}
	// Repeated scoped completion reaches the native UI but cannot publish twice.
	write(t, server, []byte(`{"method":"turn/completed","params":{"threadId":"A","turn":{"id":"task","status":"completed"}}}`))
	_ = readWithin(t, ui)
	select {
	case result := <-results:
		t.Fatalf("duplicate outcome: %+v", result)
	default:
	}
	if g.Binding().Ready {
		t.Fatal("reporting promoted the old root")
	}
}
