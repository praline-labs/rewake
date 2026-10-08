package endpoint

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/channel"
)

// told collects the channel events an endpoint tells.
type told struct {
	mu     sync.Mutex
	events []channel.Event
}

func (c *told) take(e channel.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

// wait returns the events once n were told, or fails.
func (c *told) wait(t *testing.T, n int) []channel.Event {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		got := append([]channel.Event(nil), c.events...)
		c.mu.Unlock()
		if len(got) >= n {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	t.Fatalf("%d events told, wanted %d: %+v", len(c.events), n, c.events)
	return nil
}

func channelEndpoint(t *testing.T, transport string, change ...func(*Config)) (*Endpoint, string, *told) {
	t.Helper()
	got := &told{}
	served, path := testEndpoint(t, transport, append([]func(*Config){func(c *Config) { c.Channel = got.take }}, change...)...)
	return served, path, got
}

// Each server connection has its own generation, and its close names it:
// the wrapper can tell an older server's EOF from the newer one's.
func TestServerConnectionsAreNumberedAndTheirClosesNamed(t *testing.T) {
	_, path, got := channelEndpoint(t, testTransport)
	first, err := Dial(path, "secret")
	if err != nil {
		t.Fatal(err)
	}
	// The greeting is answered before the connection takes its generation, so
	// Dial returning does not order the two; the first Hello does.
	got.wait(t, 1)
	second, err := Dial(path, "secret")
	if err != nil {
		t.Fatal(err)
	}
	got.wait(t, 2)
	first.Close()
	events := got.wait(t, 3)
	second.Close()
	events = append(events[:3:3], got.wait(t, 4)[3])
	want := []channel.Event{{Kind: channel.Hello, Generation: 1}, {Kind: channel.Hello, Generation: 2}, {Kind: channel.Closed, Generation: 1}, {Kind: channel.Closed, Generation: 2}}
	for i, e := range events {
		if e.Kind != want[i].Kind || e.Generation != want[i].Generation || e.At.Boot == 0 || e.At.Wall.IsZero() {
			t.Fatalf("event %d: %+v, want %+v", i, e, want[i])
		}
	}
}

// A server's refused hello is told with whether it came from the harness's
// tree; a child's or a hook's is not the server's and is not told.
func TestARefusedServerHelloIsToldWithItsAncestry(t *testing.T) {
	_, path, got := channelEndpoint(t, testTransport)
	_, _ = hold(t, path, hello{Role: roleChild})
	_, _ = hold(t, path, hello{Role: roleServer, Capability: "guess"})
	events := got.wait(t, 1)
	if len(events) != 1 || events[0].Kind != channel.HelloRefused || !events[0].Descendant {
		t.Fatalf("%+v", events)
	}

	_, strayPath, stray := channelEndpoint(t, testTransport, func(c *Config) {
		c.Descends = func(int, int) error { return errors.New("not below") }
	})
	_, _ = hold(t, strayPath, hello{Role: roleServer, Capability: "secret"})
	if e := stray.wait(t, 1)[0]; e.Kind != channel.HelloRefused || e.Descendant {
		t.Fatalf("a stray hello: %+v", e)
	}
}

// A server reports only that a command cannot start, in the endpoint's own
// word; anything else is refused and tells nothing.
func TestAServerReportsACommandThatCannotStart(t *testing.T) {
	served, path, got := channelEndpoint(t, testTransport)
	client, err := Dial(path, "secret")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	client.Report()
	events := got.wait(t, 2)
	if events[1].Kind != channel.CannotStart || events[1].Generation != events[0].Generation || events[1].Generation == 0 {
		t.Fatalf("the report is not told with its server's generation: %+v", events)
	}
	text, _ := json.Marshal("the server's own words")
	reply := served.answer(nil, roleServer, request{ID: 9, Op: opReport, Payload: text}, 1)
	if reply.Error == "" || len(got.wait(t, 2)) != 2 {
		t.Fatal("an unknown report was taken")
	}
	if reply := served.answer(nil, roleChild, request{ID: 9, Op: opReport, Payload: json.RawMessage(`"cannot-start"`)}, 0); reply.Error == "" {
		t.Fatal("a child reported for a server")
	}
}

// A call refused for want of an observation says the observer is gone; a
// validated ticket carries when it was issued; a call seen opens a wait.
func TestCallsTellTheirEvidence(t *testing.T) {
	served, path, got := channelEndpoint(t, testTransport)
	openTurn(served, "th", "t1")
	_, err := ticketFor(t, path, neutralRequest("th", "t1", "unseen", words))
	refusedWith(t, err, "an unobserved call")
	seenCall(served, "th", "t1", "c1", words)
	ticket := mustTicket(t, path, neutralRequest("th", "t1", "c1", words))
	if err := Confirm(path, ticket); err != nil {
		t.Fatal(err)
	}
	var calls []channel.Event
	for _, e := range got.wait(t, 7) {
		if e.Kind != channel.Hello && e.Kind != channel.Closed && e.Kind != channel.Bound {
			calls = append(calls, e)
		}
	}
	if len(calls) != 2 || calls[0].Kind != channel.NotObserved || calls[1].Kind != channel.Validated || calls[1].Issued != ticket.CalledBoot {
		t.Fatalf("%+v", calls)
	}

	hooked, _, seen := channelEndpoint(t, bridge.ClaudeTransport)
	payload, _ := json.Marshal(map[string]any{
		"hook_event_name": "PreToolUse", "tool_name": claudeToolName, "tool_use_id": "u1",
		"tool_input": map[string]any{"words": words}, "session_id": "s", "prompt_id": "p",
	})
	hooked.hook(payload)
	if e := seen.wait(t, 1)[0]; e.Kind != channel.CallSeen {
		t.Fatalf("%+v", e)
	}
}
