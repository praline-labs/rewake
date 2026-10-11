package endpoint

import (
	"errors"
	"os"
	"sync"
	"testing"
	"time"

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

// Each process the run names as its transport is a generation of the
// channel, and its end names it: naming another ends the one before, naming
// none ends it, and so does the endpoint's close — the tool transport's
// lifecycle is the channel's hello and close, so the wrapper can tell an
// older transport's end from the newer one's.
func TestEachTransportNamedIsAGenerationAndItsEndNamed(t *testing.T) {
	served, _, got := channelEndpoint(t, testTransport)
	served.SetTransport(101, 1)
	served.SetTransport(102, 2)
	served.SetTransport(0, 0)
	served.SetTransport(103, 3)
	served.Close()
	served.SetTransport(104, 4)
	events := got.wait(t, 6)
	want := []channel.Event{
		{Kind: channel.Hello, Generation: 1},
		{Kind: channel.Closed, Generation: 1},
		{Kind: channel.Hello, Generation: 2},
		{Kind: channel.Closed, Generation: 2},
		{Kind: channel.Hello, Generation: 3},
		{Kind: channel.Closed, Generation: 3},
	}
	if len(events) != len(want) {
		t.Fatalf("a transport named after the close opened a generation: %+v", events)
	}
	for i, e := range events {
		if e.Kind != want[i].Kind || e.Generation != want[i].Generation || e.At.Boot == 0 || e.At.Wall.IsZero() {
			t.Fatalf("event %d: %+v, want %+v", i, e, want[i])
		}
		if i > 0 && e.At.Boot < events[i-1].At.Boot {
			t.Fatalf("event %d stamped before the one it follows: %+v", i, events)
		}
	}
}

// A transport's refused hello is told with whether it came from the
// harness's tree; a child's is not the transport's and is not told.
func TestARefusedTransportHelloIsToldWithItsAncestry(t *testing.T) {
	served, path, got := channelEndpoint(t, testTransport)
	_, _ = hold(t, path, hello{Role: roleChild})
	// The run's transport is another process: this one, below the harness
	// as a command the model runs is, asks in its place.
	served.SetTransport(os.Getppid(), 1)
	_, _ = hold(t, path, hello{Role: roleTransport})
	events := got.wait(t, 2)
	if len(events) != 2 || events[1].Kind != channel.HelloRefused || !events[1].Descendant {
		t.Fatalf("%+v", events)
	}

	_, strayPath, stray := channelEndpoint(t, testTransport, func(c *Config) {
		c.Descends = func(int, int) error { return errors.New("not below") }
	})
	_, _ = hold(t, strayPath, hello{Role: roleTransport})
	if e := stray.wait(t, 1)[0]; e.Kind != channel.HelloRefused || e.Descendant {
		t.Fatalf("a stray hello: %+v", e)
	}
}

// A validated ticket carries when it was issued; a call refused for want of
// an observation tells nothing, since silence proves nothing.
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
	deadline := time.Now().Add(3 * time.Second)
	for len(calls) == 0 && time.Now().Before(deadline) {
		got.mu.Lock()
		for _, e := range got.events {
			if e.Kind != channel.Hello && e.Kind != channel.Closed {
				calls = append(calls, e)
			}
		}
		got.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	if len(calls) != 1 || calls[0].Kind != channel.Validated || calls[0].Issued != ticket.CalledBoot {
		t.Fatalf("%+v", calls)
	}
}
