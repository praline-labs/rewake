package endpoint

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/bridge"
)

var words = []string{"inbox"}

// A call runs only under a ticket issued after the harness's own record of
// it; without one by the end of the wait, nothing is issued.
func TestATicketNeedsTheHarnessesOwnObservation(t *testing.T) {
	served, path := testEndpoint(t, bridge.CodexTransport)
	openTurn(served, "th", "t1")
	_, err := ticketFor(t, path, codexRequest("th", "t1", "c1", words))
	refusedWith(t, err, "no observation")
	if !strings.Contains(err.Error(), "did not report") {
		t.Fatalf("the refusal: %v", err)
	}
}

// Neither harness orders its record and the server's request: either may come
// first.
func TestTheObservationMayComeEitherSideOfTheRequest(t *testing.T) {
	served, path := testEndpoint(t, bridge.CodexTransport)
	openTurn(served, "th", "t1")
	served.CodexEvent(codexEvent("item/started", "th", "t1", "before", words, nil))
	ticket := mustTicket(t, path, codexRequest("th", "t1", "before", words))
	if ticket.Conversation != "th" || ticket.Turn != "t1" || ticket.CallID != "before" || ticket.Nonce == "" || ticket.Capability != "secret" {
		t.Fatalf("the ticket: %+v", ticket)
	}
	if span := ticket.DeadlineBoot - ticket.CalledBoot; span != int64(25*time.Second) {
		t.Fatalf("the deadline is %s after the call", time.Duration(span))
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		served.CodexEvent(codexEvent("item/started", "th", "t1", "after", words, nil))
	}()
	mustTicket(t, path, codexRequest("th", "t1", "after", words))
}

// One native call gets one ticket, whichever server connection asks.
func TestOneCallGetsOneTicket(t *testing.T) {
	served, path := testEndpoint(t, bridge.CodexTransport)
	openTurn(served, "th", "t1")
	var wg sync.WaitGroup
	var mu sync.Mutex
	issued := 0
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := ticketFor(t, path, codexRequest("th", "t1", "c1", words)); err == nil {
				mu.Lock()
				issued++
				mu.Unlock()
			}
		}()
	}
	time.Sleep(20 * time.Millisecond)
	served.CodexEvent(codexEvent("item/started", "th", "t1", "c1", words, nil))
	wg.Wait()
	if issued != 1 {
		t.Fatalf("%d tickets for one call", issued)
	}
}

// Missing, conflicting or duplicate correlation refuses.
func TestConflictingCorrelationIsRefused(t *testing.T) {
	served, path := testEndpoint(t, bridge.CodexTransport)
	openTurn(served, "th", "t1")
	cases := []struct {
		name    string
		observe func(call string)
		asked   func(call string) TicketRequest
	}{
		{"another turn in _meta", func(c string) { served.CodexEvent(codexEvent("item/started", "th", "t1", c, words, nil)) }, func(c string) TicketRequest { return codexRequest("th", "t9", c, words) }},
		{"another thread in _meta", func(c string) { served.CodexEvent(codexEvent("item/started", "th", "t1", c, words, nil)) }, func(c string) TicketRequest { return codexRequest("other", "t1", c, words) }},
		{"other words", func(c string) { served.CodexEvent(codexEvent("item/started", "th", "t1", c, words, nil)) }, func(c string) TicketRequest { return codexRequest("th", "t1", c, []string{"whoami"}) }},
		{"reported twice", func(c string) {
			served.CodexEvent(codexEvent("item/started", "th", "t1", c, words, nil))
			served.CodexEvent(codexEvent("item/started", "th", "t1", c, words, nil))
		}, func(c string) TicketRequest { return codexRequest("th", "t1", c, words) }},
		{"a turn never seen to start", func(c string) { served.CodexEvent(codexEvent("item/started", "th", "t7", c, words, nil)) }, func(c string) TicketRequest { return codexRequest("th", "t7", c, words) }},
		{"another transport", func(c string) { served.CodexEvent(codexEvent("item/started", "th", "t1", c, words, nil)) }, func(c string) TicketRequest {
			asked := codexRequest("th", "t1", c, words)
			asked.Transport = bridge.ClaudeTransport
			return asked
		}},
		{"arguments of another shape", func(c string) {
			served.CodexEvent([]byte(`{"method":"item/started","params":{"threadId":"th","turnId":"t1","item":{"type":"mcpToolCall","id":"` + c + `","server":"rewake","tool":"rewake","arguments":{"words":["inbox"],"more":1}}}}`))
		}, func(c string) TicketRequest { return codexRequest("th", "t1", c, words) }},
	}
	for i, test := range cases {
		call := "conflict-" + string(rune('a'+i))
		test.observe(call)
		_, err := ticketFor(t, path, test.asked(call))
		refusedWith(t, err, test.name)
	}
	// A completed turn takes no more calls.
	served.CodexEvent(codexEvent("item/started", "th", "t1", "late", words, nil))
	served.CodexEvent(codexEvent("turn/completed", "th", "t1", "", nil, nil))
	_, err := ticketFor(t, path, codexRequest("th", "t1", "late", words))
	refusedWith(t, err, "a completed turn")
}

// A ticket is confirmed once, for one process, as it was issued, and never
// after its deadline.
func TestAConfirmationIsOneTime(t *testing.T) {
	served, path := testEndpoint(t, bridge.CodexTransport)
	openTurn(served, "th", "t1")
	served.CodexEvent(codexEvent("item/started", "th", "t1", "c1", words, nil))
	ticket := mustTicket(t, path, codexRequest("th", "t1", "c1", words))
	forged := ticket
	forged.Turn = "t2"
	if err := Confirm(path, forged); err == nil {
		t.Fatal("a changed ticket was confirmed")
	}
	if err := Confirm(path, ticket); err != nil {
		t.Fatalf("the ticket was not confirmed: %v", err)
	}
	if err := Confirm(path, ticket); err == nil || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("a second confirmation: %v", err)
	}
	invented := ticket
	invented.Nonce = "invented"
	if err := Confirm(path, invented); err == nil {
		t.Fatal("a ticket nobody issued was confirmed")
	}
	if held := served.calls.byNonce[ticket.Nonce]; held.pid == 0 {
		t.Fatal("the process that used the ticket is not recorded")
	}

	short, shortPath := testEndpoint(t, bridge.CodexTransport, func(c *Config) { c.Span = time.Millisecond })
	openTurn(short, "th", "t1")
	short.CodexEvent(codexEvent("item/started", "th", "t1", "c1", words, nil))
	expired := mustTicket(t, shortPath, codexRequest("th", "t1", "c1", words))
	for boottime.Now() < expired.DeadlineBoot {
		time.Sleep(time.Millisecond)
	}
	if err := Confirm(shortPath, expired); err == nil || !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("an expired ticket: %v", err)
	}
}

// The deadline of a ticket, and the refusal of a transport timeout too short
// to run a call in.
func TestTheDeadlineFollowsTheTransportsTimeout(t *testing.T) {
	env := func(value string) func(string) string {
		return func(name string) string {
			if name == "MCP_TOOL_TIMEOUT" {
				return value
			}
			return ""
		}
	}
	cases := []struct {
		transport, setting string
		span               time.Duration
		refused            bool
	}{
		{bridge.CodexTransport, "1000", 25 * time.Second, false},
		{bridge.ClaudeTransport, "", 25 * time.Second, false},
		{bridge.ClaudeTransport, "60000", 25 * time.Second, false},
		{bridge.ClaudeTransport, "10000", 8 * time.Second, false},
		{bridge.ClaudeTransport, "7000", 5 * time.Second, false},
		{bridge.ClaudeTransport, "6000", 4 * time.Second, true},
	}
	for _, test := range cases {
		span, refusal := DeadlineFor(test.transport, env(test.setting))
		if span != test.span || (refusal != "") != test.refused || test.refused && !strings.Contains(refusal, "MCP_TOOL_TIMEOUT") {
			t.Fatalf("%s %q: %s %q", test.transport, test.setting, span, refusal)
		}
	}
	served, path := testEndpoint(t, bridge.CodexTransport, func(c *Config) { c.Refusal = "the setting names its reason" })
	openTurn(served, "th", "t1")
	served.CodexEvent(codexEvent("item/started", "th", "t1", "c1", words, nil))
	if _, err := ticketFor(t, path, codexRequest("th", "t1", "c1", words)); err == nil || !strings.Contains(err.Error(), "names its reason") {
		t.Fatalf("a run whose deadline is too short: %v", err)
	}
}

// Without a wait of its own, the endpoint waits two seconds for the
// harness's record of a call, and no more (rule 9).
func TestTheObservationWaitIsTwoSeconds(t *testing.T) {
	served, path := testEndpoint(t, bridge.CodexTransport, func(c *Config) { c.Wait = 0 })
	openTurn(served, "th", "t1")
	start := time.Now()
	_, err := ticketFor(t, path, codexRequest("th", "t1", "missing", words))
	if took := time.Since(start); err == nil || took < 1900*time.Millisecond || took > 2500*time.Millisecond {
		t.Fatalf("refused after %s: %v", took, err)
	}
}
