package endpoint

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/harness"
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

// testTransport is a transport of no harness: the endpoint's turn path for
// every transport but the hook path, whose hooks name a call's turn.
const testTransport = "test-transport"

// neutralRequest is a transport's ask for the ticket of a call it binds to
// the harness's own thread and turn, declaring that its turn ids are never
// reused.
func neutralRequest(thread, turn, call string, words []string) TicketRequest {
	return TicketRequest{Transport: testTransport, Conversation: thread, Turn: turn, CallID: call, Words: words, Digest: bridge.Digest(words), TurnsNeverReused: true}
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

// openTurn starts a turn on thread through the neutral input.
func openTurn(served *Endpoint, thread, turn string) { served.TurnStarted(thread, turn, 0) }

// seenCall reports a call through the neutral input, as an adapter does.
func seenCall(served *Endpoint, thread, turn, call string, words []string) {
	served.CallSeen(harness.ObservedCall{ID: call, Conversation: thread, Turn: turn, Words: words})
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
