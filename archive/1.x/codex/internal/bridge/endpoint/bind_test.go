package endpoint

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/channel"
)

// The first request of a Codex server's connection that names a thread binds
// that connection to it, told once with its generation; a later request of
// the same connection naming another thread binds nothing
// (docs/mail-bridge-channel-codex.md#conversation-connections).
func TestTheFirstThreadAConnectionNamesBindsIt(t *testing.T) {
	served, path, got := channelEndpoint(t, bridge.CodexTransport, func(c *Config) { c.Wait = 5 * time.Second })
	served.SetPrimary(func() string { return "A" })
	client, err := Dial(path, "secret")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	got.wait(t, 1)
	_, err = client.Ticket(codexRequest("", "t1", "c0", words), 3*time.Second)
	refusedWith(t, err, "a call naming no thread")
	_, err = client.Ticket(codexRequest("sub", "t1", "c1", words), 3*time.Second)
	refusedWith(t, err, "a sub-agent's call")
	_, err = client.Ticket(codexRequest("other", "t1", "c2", words), 3*time.Second)
	refusedWith(t, err, "another thread's call")
	// Past the refusals, so a second binding would have been told by now.
	time.Sleep(50 * time.Millisecond)
	events := got.wait(t, 2)
	if len(events) != 2 || events[1].Kind != channel.Bound || events[1].Generation != 1 || events[1].Thread != "sub" || events[1].At.Boot == 0 {
		t.Fatalf("%+v", events)
	}
}

// A request naming no thread or another than the primary is refused at once:
// the gateway forwards no other thread's items, so it is never left to wait
// and never told as an unobserved call — a sub-agent's call is not the
// conversation's fault.
func TestAnotherThreadsCallIsRefusedBeforeAnyWait(t *testing.T) {
	primary := "A"
	served, path, got := channelEndpoint(t, bridge.CodexTransport, func(c *Config) { c.Wait = 5 * time.Second })
	served.SetPrimary(func() string { return primary })
	openTurn(served, "A", "t1")
	for _, thread := range []string{"", "sub"} {
		began := time.Now()
		_, err := ticketFor(t, path, codexRequest(thread, "t1", "c-"+thread, words))
		refusedWith(t, err, "thread "+thread)
		if waited := time.Since(began); waited > 2*time.Second {
			t.Fatalf("thread %q: refused after %v, past the wait", thread, waited)
		}
	}
	// While a selection is pending the primary is empty: every call is
	// another thread's.
	primary = ""
	_, err := ticketFor(t, path, codexRequest("A", "t1", "c-pending", words))
	refusedWith(t, err, "a call while no thread is primary")
	for _, e := range got.wait(t, 1) {
		if e.Kind == channel.NotObserved {
			t.Fatalf("a refused thread's call was told as unobserved: %+v", e)
		}
	}
}

// The primary's own call is matched as before, binding its connection.
func TestThePrimarysCallIsMatchedAndBinds(t *testing.T) {
	served, path, got := channelEndpoint(t, bridge.CodexTransport)
	served.SetPrimary(func() string { return "A" })
	openTurn(served, "A", "t1")
	served.CodexEvent(codexEvent("item/started", "A", "t1", "c1", words, nil))
	mustTicket(t, path, codexRequest("A", "t1", "c1", words))
	bound := 0
	for _, e := range got.wait(t, 2) {
		if e.Kind == channel.Bound && e.Thread == "A" {
			bound++
		}
	}
	if bound != 1 {
		t.Fatalf("the primary's connection was bound %d times", bound)
	}
}

// Claude Code's servers serve the one conversation: nothing binds there.
func TestClaudeCodesRequestsBindNothing(t *testing.T) {
	served, _, got := channelEndpoint(t, bridge.ClaudeTransport)
	served.SetPrimary(func() string { return "A" })
	_, _ = served.issue(TicketRequest{Transport: bridge.ClaudeTransport, Conversation: "x", CallID: "u1"}, 1)
	for _, e := range got.wait(t, 1) {
		if e.Kind == channel.Bound && e.Thread == "x" {
			t.Fatalf("a Claude Code request bound its connection: %+v", e)
		}
	}
}

// A failed startup status carries the thread it names, or none: only the
// primary's, or one naming none, fails the conversation's channel.
func TestAFailedStartupStatusCarriesItsThread(t *testing.T) {
	served, _, got := channelEndpoint(t, bridge.CodexTransport)
	served.CodexEvent([]byte(`{"method":"mcpServer/startupStatus/updated","params":{"name":"rewake","status":"failed","threadId":"B","error":"words"}}`))
	served.CodexEvent([]byte(`{"method":"mcpServer/startupStatus/updated","params":{"name":"rewake","status":"failed"}}`))
	events := got.wait(t, 2)
	if events[0].Kind != channel.StartupFailed || events[0].Thread != "B" || events[1].Thread != "" || events[0].Class != "" {
		t.Fatalf("%+v", events)
	}
}

// A wait admitted for the primary may outlive it: another conversation is
// selected meanwhile. What the call ends with — no observation, or a ticket
// its child validates — is still its own connection's and thread's, so the
// channel judges it against the conversation it was admitted for
// (docs/mail-bridge-channel-codex.md#as-built).
func TestAWaitOutlivingThePrimaryKeepsItsConnection(t *testing.T) {
	for _, observed := range []bool{false, true} {
		var primary atomic.Value
		primary.Store("A")
		served, path, got := channelEndpoint(t, bridge.CodexTransport, func(c *Config) { c.Wait = 300 * time.Millisecond })
		served.SetPrimary(func() string { return primary.Load().(string) })
		openTurn(served, "A", "t1")
		client, err := Dial(path, "secret")
		if err != nil {
			t.Fatal(err)
		}
		issued := make(chan bridge.Ticket, 1)
		go func() {
			ticket, _ := client.Ticket(codexRequest("A", "t1", "c1", words), 3*time.Second)
			issued <- ticket
		}()
		got.wait(t, 2) // the hello and the binding: the call passed its check
		primary.Store("B")
		if observed {
			served.CodexEvent(codexEvent("item/started", "A", "t1", "c1", words, nil))
		}
		ticket := <-issued
		client.Close()
		want := channel.NotObserved
		if observed {
			want = channel.Validated
			if err := Confirm(path, ticket); err != nil {
				t.Fatalf("the ticket issued after the primary changed: %v", err)
			}
		}
		var outcome *channel.Event
		for _, e := range got.wait(t, 4) {
			if e.Kind == want {
				outcome = &e
			}
		}
		if outcome == nil || outcome.Generation != 1 || outcome.Thread != "A" {
			t.Fatalf("observed %v: the call's outcome lost its connection or thread: %+v", observed, outcome)
		}
	}
}
