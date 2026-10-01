package endpoint

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
)

// testEndpoint serves a run in a temporary state directory, as the wrapper
// does, with the build check stood in for: the test is the only process.
func testEndpoint(t *testing.T, transport string, change ...func(*Config)) (*Endpoint, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "ep")
	if err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(dir, 0o700)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	cfg := Config{
		Dir: dir, Name: "api", Epoch: "e1", Transport: transport, Capability: "secret",
		Span:      25 * time.Second,
		Words:     func(words []string) ([]string, error) { return words, nil },
		Wait:      300 * time.Millisecond,
		SameBuild: func(int) error { return nil },
	}
	for _, apply := range change {
		apply(&cfg)
	}
	path := filepath.Join(dir, "api.ctx")
	served, err := Listen(path, cfg)
	if err != nil {
		t.Fatal(err)
	}
	served.SetRoots(os.Getpid())
	t.Cleanup(served.Close)
	return served, path
}

// codexEvent is a Codex notification in the shape of the recorded stream
// (testdata/codex-stream.jsonl).
func codexEvent(method, thread, turn, call string, words []string, content any) []byte {
	params := map[string]any{"threadId": thread}
	switch method {
	case "turn/started", "turn/completed":
		params["turn"] = map[string]any{"id": turn, "status": "inProgress"}
	default:
		status := "inProgress"
		var result any
		if method == "item/completed" {
			status, result = "completed", map[string]any{"content": content}
		}
		params["turnId"] = turn
		params["item"] = map[string]any{
			"type": "mcpToolCall", "id": call, "server": "rewake", "tool": "rewake", "status": status,
			"arguments": map[string]any{"words": words}, "error": nil, "result": result,
		}
	}
	encoded, _ := json.Marshal(map[string]any{"method": method, "params": params})
	return encoded
}

// request is the server's ask for the ticket of a Codex call.
func codexRequest(thread, turn, call string, words []string) TicketRequest {
	return TicketRequest{Transport: bridge.CodexTransport, Conversation: thread, Turn: turn, CallID: call, Words: words, Digest: bridge.Digest(words)}
}

// ticketFor asks for a ticket over a server's connection.
func ticketFor(t *testing.T, path string, asked TicketRequest) (bridge.Ticket, error) {
	t.Helper()
	client, err := Dial(path, "secret")
	if err != nil {
		return bridge.Ticket{}, err
	}
	defer client.Close()
	return client.Ticket(asked, 3*time.Second)
}

// openTurn starts a Codex turn on thread.
func openTurn(served *Endpoint, thread, turn string) {
	served.CodexEvent(codexEvent("turn/started", thread, turn, "", nil, nil))
}

func mustTicket(t *testing.T, path string, asked TicketRequest) bridge.Ticket {
	t.Helper()
	ticket, err := ticketFor(t, path, asked)
	if err != nil {
		t.Fatalf("no ticket: %v", err)
	}
	return ticket
}

func refusedWith(t *testing.T, err error, what string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: issued", what)
	}
	if errors.Is(err, ErrUnreachable) {
		t.Fatalf("%s: unreachable rather than refused: %v", what, err)
	}
}
