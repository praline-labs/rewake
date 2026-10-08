package endpoint

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/harness"
)

var words = []string{"inbox"}

// A call runs only under a ticket issued after the harness's own record of
// it; without one by the end of the wait, nothing is issued.
func TestATicketNeedsTheHarnessesOwnObservation(t *testing.T) {
	served, path := testEndpoint(t, testTransport)
	openTurn(served, "th", "t1")
	_, err := ticketFor(t, path, neutralRequest("th", "t1", "c1", words))
	refusedWith(t, err, "no observation")
	if !strings.Contains(err.Error(), "did not report") {
		t.Fatalf("the refusal: %v", err)
	}
}

// Neither harness orders its record and the server's request: either may come
// first.
func TestTheObservationMayComeEitherSideOfTheRequest(t *testing.T) {
	served, path := testEndpoint(t, testTransport)
	openTurn(served, "th", "t1")
	seenCall(served, "th", "t1", "before", words)
	ticket := mustTicket(t, path, neutralRequest("th", "t1", "before", words))
	if ticket.Conversation != "th" || ticket.Turn != "t1" || ticket.CallID != "before" || ticket.Nonce == "" || ticket.Capability != "secret" {
		t.Fatalf("the ticket: %+v", ticket)
	}
	if span := ticket.DeadlineBoot - ticket.CalledBoot; span != int64(25*time.Second) {
		t.Fatalf("the deadline is %s after the call", time.Duration(span))
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		seenCall(served, "th", "t1", "after", words)
	}()
	mustTicket(t, path, neutralRequest("th", "t1", "after", words))
}

// One native call gets one ticket, whichever server connection asks.
func TestOneCallGetsOneTicket(t *testing.T) {
	served, path := testEndpoint(t, testTransport)
	openTurn(served, "th", "t1")
	var wg sync.WaitGroup
	var mu sync.Mutex
	issued := 0
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := ticketFor(t, path, neutralRequest("th", "t1", "c1", words)); err == nil {
				mu.Lock()
				issued++
				mu.Unlock()
			}
		}()
	}
	time.Sleep(20 * time.Millisecond)
	seenCall(served, "th", "t1", "c1", words)
	wg.Wait()
	if issued != 1 {
		t.Fatalf("%d tickets for one call", issued)
	}
}

// Missing, conflicting or duplicate correlation refuses.
func TestConflictingCorrelationIsRefused(t *testing.T) {
	served, path := testEndpoint(t, testTransport)
	openTurn(served, "th", "t1")
	cases := []struct {
		name    string
		observe func(call string)
		asked   func(call string) TicketRequest
	}{
		{"another turn in the binding", func(c string) { seenCall(served, "th", "t1", c, words) }, func(c string) TicketRequest { return neutralRequest("th", "t9", c, words) }},
		{"another thread in the binding", func(c string) { seenCall(served, "th", "t1", c, words) }, func(c string) TicketRequest { return neutralRequest("other", "t1", c, words) }},
		{"other words", func(c string) { seenCall(served, "th", "t1", c, words) }, func(c string) TicketRequest { return neutralRequest("th", "t1", c, []string{"whoami"}) }},
		{"reported twice", func(c string) {
			seenCall(served, "th", "t1", c, words)
			seenCall(served, "th", "t1", c, words)
		}, func(c string) TicketRequest { return neutralRequest("th", "t1", c, words) }},
		{"a turn never seen to start", func(c string) { seenCall(served, "th", "t7", c, words) }, func(c string) TicketRequest { return neutralRequest("th", "t7", c, words) }},
		{"another transport", func(c string) { seenCall(served, "th", "t1", c, words) }, func(c string) TicketRequest {
			asked := neutralRequest("th", "t1", c, words)
			asked.Transport = "another-transport"
			return asked
		}},
		{"arguments that are not words", func(c string) {
			served.CallSeen(harness.ObservedCall{ID: c, Conversation: "th", Turn: "t1"})
		}, func(c string) TicketRequest { return neutralRequest("th", "t1", c, words) }},
	}
	for i, test := range cases {
		call := "conflict-" + string(rune('a'+i))
		test.observe(call)
		_, err := ticketFor(t, path, test.asked(call))
		refusedWith(t, err, test.name)
	}
	// A completed turn takes no more calls.
	seenCall(served, "th", "t1", "late", words)
	served.TurnEnded("th", "t1")
	_, err := ticketFor(t, path, neutralRequest("th", "t1", "late", words))
	refusedWith(t, err, "a completed turn")
}

// A ticket is confirmed once, for one process, as it was issued, and never
// after its deadline.
func TestAConfirmationIsOneTime(t *testing.T) {
	served, path := testEndpoint(t, testTransport)
	openTurn(served, "th", "t1")
	seenCall(served, "th", "t1", "c1", words)
	ticket := mustTicket(t, path, neutralRequest("th", "t1", "c1", words))
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

	short, shortPath := testEndpoint(t, testTransport, func(c *Config) { c.Span = time.Millisecond })
	openTurn(short, "th", "t1")
	seenCall(short, "th", "t1", "c1", words)
	expired := mustTicket(t, shortPath, neutralRequest("th", "t1", "c1", words))
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
			if name == "TOOL_TIMEOUT" {
				return value
			}
			return ""
		}
	}
	cases := []struct {
		variable, setting string
		span              time.Duration
		refused           bool
	}{
		{"", "1000", 25 * time.Second, false},
		{"TOOL_TIMEOUT", "", 25 * time.Second, false},
		{"TOOL_TIMEOUT", "60000", 25 * time.Second, false},
		{"TOOL_TIMEOUT", "10000", 8 * time.Second, false},
		{"TOOL_TIMEOUT", "7000", 5 * time.Second, false},
		{"TOOL_TIMEOUT", "6000", 4 * time.Second, true},
	}
	for _, test := range cases {
		span, refusal := DeadlineFor(test.variable, env(test.setting))
		if span != test.span || (refusal != "") != test.refused || test.refused && !strings.Contains(refusal, "TOOL_TIMEOUT=6000") {
			t.Fatalf("%s %q: %s %q", test.variable, test.setting, span, refusal)
		}
	}
	served, path := testEndpoint(t, testTransport, func(c *Config) { c.Refusal = "the setting names its reason" })
	openTurn(served, "th", "t1")
	seenCall(served, "th", "t1", "c1", words)
	if _, err := ticketFor(t, path, neutralRequest("th", "t1", "c1", words)); err == nil || !strings.Contains(err.Error(), "names its reason") {
		t.Fatalf("a run whose deadline is too short: %v", err)
	}
}

// Without a wait of its own, the endpoint waits two seconds for the
// harness's record of a call, and no more (rule 9).
func TestTheObservationWaitIsTwoSeconds(t *testing.T) {
	served, path := testEndpoint(t, testTransport, func(c *Config) { c.Wait = 0 })
	openTurn(served, "th", "t1")
	start := time.Now()
	_, err := ticketFor(t, path, neutralRequest("th", "t1", "missing", words))
	if took := time.Since(start); err == nil || took < 1900*time.Millisecond || took > 2500*time.Millisecond {
		t.Fatalf("refused after %s: %v", took, err)
	}
}
