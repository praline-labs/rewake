package endpoint

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/bridge"
)

// A ticket is issued only while no end was noted since its call was heard,
// or, where the binding names the turn, since its turn started: the capture
// closes the turn for new tickets before the turn's end reaches the table.

// A turn whose end was captured, with the harness's report of its end not yet
// heard, gives a call of it reported afterwards no ticket.
func TestACapturedTurnIssuesNoMoreTickets(t *testing.T) {
	served, path := testEndpoint(t, testTransport)
	openTurn(served, "th", "t1")
	served.Gate().Capture()
	seenCall(served, "th", "t1", "late", words)
	_, err := ticketFor(t, path, neutralRequest("th", "t1", "late", words))
	if err == nil || !strings.Contains(err.Error(), "ended before its ticket") {
		t.Fatalf("a call of a captured turn: %v", err)
	}
	// The next turn starts after the end, and its calls are served.
	openTurn(served, "th", "t2")
	seenCall(served, "th", "t2", "next", words)
	mustTicket(t, path, neutralRequest("th", "t2", "next", words))
}

// On Claude Code an interruption is the captured end: a call heard before it
// gets no ticket after it, and a call heard after it — a prompt that goes on
// past an end another hook blocked — does.
func TestAClaudeCallHeardBeforeACaptureGetsNoTicketAfterIt(t *testing.T) {
	served, path := testEndpoint(t, bridge.ClaudeTransport, func(c *Config) { c.Conversation = func() string { return "conv" } })
	pre := func(call string) TicketRequest {
		served.hook([]byte(`{"session_id":"conv","prompt_id":"p1","hook_event_name":"PreToolUse","tool_name":"mcp__rewake__rewake","tool_input":{"words":["inbox"]},"tool_use_id":"` + call + `","mcp_server":{"name":"rewake"}}`))
		return TicketRequest{Transport: bridge.ClaudeTransport, CallID: call, Words: words, Digest: bridge.Digest(words)}
	}
	early := pre("early")
	served.Gate().Capture()
	if _, err := ticketFor(t, path, early); err == nil || !strings.Contains(err.Error(), "ended before its ticket") {
		t.Fatalf("a call heard before the capture: %v", err)
	}
	mustTicket(t, path, pre("after"))
}

// A run remembers every call it gave a ticket, up to maxSpent; past that it
// refuses, rather than forget one and serve it twice.
func TestARunPastTheTicketsItRemembersRefuses(t *testing.T) {
	served, path := testEndpoint(t, testTransport)
	openTurn(served, "th", "t1")
	served.calls.mu.Lock()
	for n := range maxSpent {
		served.calls.spent["spent-"+strconv.Itoa(n)] = true
	}
	served.calls.mu.Unlock()
	seenCall(served, "th", "t1", "one-more", words)
	_, err := ticketFor(t, path, neutralRequest("th", "t1", "one-more", words))
	if err == nil || !strings.Contains(err.Error(), "all the tickets it can remember") {
		t.Fatalf("a ticket past the run's memory: %v", err)
	}
}

// The last ticket a run can remember goes to one of two calls asking at
// once, and stays spent when its call ages, is pushed out of the table and
// is heard again.
func TestTheLastTicketARunRemembersIsIssuedOnce(t *testing.T) {
	served, path := testEndpoint(t, testTransport)
	openTurn(served, "th", "t1")
	served.calls.mu.Lock()
	for n := range maxSpent - 1 {
		served.calls.spent["spent-"+strconv.Itoa(n)] = true
	}
	served.calls.mu.Unlock()
	calls := []string{"last-a", "last-b"}
	for _, call := range calls {
		seenCall(served, "th", "t1", call, words)
	}
	start := make(chan struct{})
	results := make(chan asked, len(calls))
	var both sync.WaitGroup
	for _, call := range calls {
		both.Add(1)
		go func() {
			defer both.Done()
			<-start
			ticket, err := ticketFor(t, path, neutralRequest("th", "t1", call, words))
			results <- asked{ticket, err}
		}()
	}
	close(start)
	both.Wait()
	close(results)
	var last bridge.Ticket
	issued := 0
	for got := range results {
		switch {
		case got.err == nil:
			issued++
			last = got.ticket
		case !strings.Contains(got.err.Error(), "all the tickets it can remember"):
			t.Fatal(got.err)
		}
	}
	if issued != 1 {
		t.Fatalf("the last place issued %d tickets", issued)
	}
	if err := Confirm(path, last); err != nil {
		t.Fatalf("the last ticket's confirmation: %v", err)
	}
	served.calls.mu.Lock()
	served.calls.now = func() time.Time { return time.Now().Add(callLife + time.Second) }
	served.calls.mu.Unlock()
	for n := range maxCalls + 1 {
		seenCall(served, "th", "t1", fmt.Sprintf("pressure-%d", n), words)
	}
	seenCall(served, "th", "t1", last.CallID, words)
	if _, err := ticketFor(t, path, neutralRequest("th", "t1", last.CallID, words)); err == nil {
		t.Fatal("the last ticket was issued again after its call left the table")
	}
	served.calls.mu.Lock()
	defer served.calls.mu.Unlock()
	if len(served.calls.spent) != maxSpent || !served.calls.spent[last.CallID] {
		t.Fatalf("the run remembers %d tickets", len(served.calls.spent))
	}
	if len(served.calls.byCall) > maxCalls {
		t.Fatalf("the table grew to %d calls", len(served.calls.byCall))
	}
}

// A capture that noted its end and waits for an acknowledgment still writing
// already refuses tickets, before it returns.
func TestNoTicketWhileACaptureWaitsForAWriter(t *testing.T) {
	served, path := testEndpoint(t, testTransport)
	openTurn(served, "th", "t1")
	seenCall(served, "th", "t1", "late", words)
	leave, ok := served.Gate().Enter(boottime.Now())
	if !ok {
		t.Fatal("the writing acknowledgment was refused")
	}
	defer leave()
	captured := make(chan struct{})
	go func() {
		served.Gate().Capture()
		close(captured)
	}()
	for until := time.Now().Add(time.Second); served.Gate().Noted() == 0; time.Sleep(time.Millisecond) {
		if time.Now().After(until) {
			t.Fatal("the capture never noted its end")
		}
	}
	if _, err := ticketFor(t, path, neutralRequest("th", "t1", "late", words)); err == nil || !strings.Contains(err.Error(), "ended before its ticket") {
		t.Fatalf("a ticket while the capture waits: %v", err)
	}
	select {
	case <-captured:
		t.Fatal("the capture returned while the acknowledgment still wrote")
	default:
	}
	leave()
	select {
	case <-captured:
	case <-time.After(time.Second):
		t.Fatal("the capture did not return once the acknowledgment left")
	}
}
