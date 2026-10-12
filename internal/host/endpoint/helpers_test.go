package endpoint

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/receipt"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
	"github.com/praline-labs/rewake/internal/tool"
)

// testEndpoint serves a run in a room of a temporary state directory, its
// lease held, as the wrapper does, with the build check stood in for: the test is the only process.
func testEndpoint(t *testing.T, transport string, change ...func(*Config)) (*Endpoint, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "ep")
	if err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(dir, 0o700)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	// A base of a short name keeps the socket's path inside its limit.
	dir = buildtest.Room(t, dir, state.DefaultRoom)
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
	byPath.Store(path, served)
	t.Cleanup(func() { byPath.Delete(path) })
	return served, path
}

// byPath finds a test's endpoint by its socket, for the helpers that take
// the path a child or a transport would dial.
var byPath sync.Map

// testTransport is a transport of no harness, on the endpoint's one turn path.
const testTransport = "test-transport"

// neutralRequest is a transport's ask for the ticket of a call it binds to
// the harness's own thread and turn, declaring that its turn ids are never
// reused.
func neutralRequest(thread, turn, call string, words []string) TicketRequest {
	return TicketRequest{Transport: testTransport, Conversation: thread, Turn: turn, CallID: call, Words: words, Digest: tool.Digest(words), TurnsNeverReused: true}
}

// ticketFor has the endpoint at path issue the ticket of a call, as it
// does for a transport's call before running it: the rules of a ticket are
// the endpoint's, whoever brings the call.
func ticketFor(t *testing.T, path string, asked TicketRequest) (tool.Ticket, error) {
	t.Helper()
	served, ok := byPath.Load(path)
	if !ok {
		t.Fatalf("no endpoint serves %s", path)
	}
	return served.(*Endpoint).issue(asked)
}

// openTurn starts a turn on thread through the neutral input.
func openTurn(served *Endpoint, thread, turn string) { served.TurnStarted(thread, turn) }

// seenCall reports a call through the neutral input, as an adapter does.
func seenCall(served *Endpoint, thread, turn, call string, words []string) {
	served.CallSeen(adapter.ObservedCall{ID: call, Conversation: thread, Turn: turn, Words: words})
}

func mustTicket(t *testing.T, path string, asked TicketRequest) tool.Ticket {
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

// acknowledged collects what the endpoint handed the acknowledgment.
type acknowledged struct {
	mu     sync.Mutex
	tokens []string
	seen   []tool.Exposure
}

func (a *acknowledged) record(_, _, _, token string, evidence tool.Exposure, gate tool.EndGate) error {
	if gate == nil {
		return errors.New("no gate")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tokens = append(a.tokens, token)
	a.seen = append(a.seen, evidence)
	return nil
}

// used issues and confirms the ticket of a call, and binds it to a record as
// the child's CLI does.
func used(t *testing.T, served *Endpoint, path string, asked TicketRequest, token string) {
	t.Helper()
	ticket := mustTicket(t, path, asked)
	if err := Confirm(path, ticket); err != nil {
		t.Fatal(err)
	}
	if token == "" {
		return
	}
	cfg := served.cfg
	if err := receipt.Bind(cfg.Dir, cfg.Name, cfg.Epoch, tool.CallKey(ticket.Transport, ticket.Conversation, ticket.CallID), token); err != nil {
		t.Fatal(err)
	}
}

func token(n int) string { return strings.Repeat("0", 23) + string(rune('a'+n)) }
