package endpoint

import (
	"testing"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/receipt"
)

// Only a call whose ticket a child used, and whose binding names a record,
// reaches the acknowledgment — driven through the neutral input, as every
// harness's reports reach the endpoint.
func TestOnlyAUsedTicketWithItsBindingIsAcknowledged(t *testing.T) {
	var got acknowledged
	served, path := testEndpoint(t, testTransport, func(c *Config) { c.Acknowledge = got.record })
	openTurn(served, "th", "t1")
	result := harness.ToolResult{Succeeded: true, Direct: true, Texts: []string{"a letter"}}
	seen := func(call string) {
		served.CallSeen(harness.ObservedCall{ID: call, Conversation: "th", Turn: "t1", Words: words})
	}
	seen("never-asked")
	served.CallResult("never-asked", result)
	seen("not-used")
	mustTicket(t, path, neutralRequest("th", "t1", "not-used", words))
	// Bound all the same, so only the unused ticket keeps it out.
	cfg := served.cfg
	if err := receipt.Bind(cfg.Dir, cfg.Name, cfg.Epoch, bridge.CallKey(testTransport, "th", "not-used"), token(1)); err != nil {
		t.Fatal(err)
	}
	served.CallResult("not-used", result)
	seen("unbound")
	used(t, served, path, neutralRequest("th", "t1", "unbound", words), "")
	served.CallResult("unbound", result)
	seen("bound")
	used(t, served, path, neutralRequest("th", "t1", "bound", words), token(0))
	served.CallResult("bound", result)
	served.Close()
	if len(got.seen) != 1 || got.seen[0].CallID != "bound" || string(got.seen[0].Answer) != "a letter" {
		t.Fatalf("acknowledged %+v", got.seen)
	}
}

// What a result handed through the neutral input proves: its first text is
// the answer, its size the whole result's as the encoder frames it, and a
// result shortened or without text proves no answer.
func TestTheWholeResultAndItsSize(t *testing.T) {
	cases := []struct {
		result harness.ToolResult
		answer string
		size   int
	}{
		{harness.ToolResult{Texts: []string{"first", "second"}}, "first", bridge.EncodedSize("first", "second")},
		{harness.ToolResult{Texts: []string{"only"}}, "only", bridge.EncodedSize("only", "")},
		{harness.ToolResult{Texts: []string{"first", "second"}, Shortened: true}, "", 0},
		{harness.ToolResult{}, "", 0},
	}
	for _, test := range cases {
		test.result.Succeeded, test.result.Direct = true, true
		evidence := exposure("c", test.result)
		if evidence.Shortened != (test.answer == "") || string(evidence.Answer) != test.answer || evidence.ResultBytes != test.size {
			t.Fatalf("%+v: %+v", test.result, evidence)
		}
		if evidence.CallID != "c" || !evidence.Succeeded || !evidence.Direct {
			t.Fatalf("%+v: the call's own facts were lost: %+v", test.result, evidence)
		}
	}
}

// The ticket carries the transport's declaration that its turn ids are never
// reused exactly as its binding made it: declared, and absent.
func TestATicketCarriesItsTransportsDeclaration(t *testing.T) {
	for _, declared := range []bool{true, false} {
		served, path := testEndpoint(t, testTransport)
		openTurn(served, "th", "t1")
		seenCall(served, "th", "t1", "c1", words)
		asked := neutralRequest("th", "t1", "c1", words)
		asked.TurnsNeverReused = declared
		if ticket := mustTicket(t, path, asked); ticket.TurnsNeverReused != declared || ticket.Transport != testTransport {
			t.Fatalf("declared %v: the ticket says %+v", declared, ticket)
		}
	}
}

// A turn start the harness timed is recorded in the mailbox, as the latest
// start a pending mark is judged by; one without a time records nothing.
func TestATimedTurnStartIsRecordedInTheMailbox(t *testing.T) {
	served, _ := testEndpoint(t, testTransport)
	cfg := served.cfg
	served.TurnStarted("th", "t1", 0)
	if got, err := inbox.LatestTurnStart(cfg.Dir, cfg.Name, cfg.Epoch); got != 0 || err != nil {
		t.Fatalf("a start without a time: %d %v", got, err)
	}
	served.TurnStarted("th", "t2", 500)
	served.TurnStarted("th", "t3", 900)
	if got, err := inbox.LatestTurnStart(cfg.Dir, cfg.Name, cfg.Epoch); got != 900 || err != nil {
		t.Fatalf("the latest start: %d %v", got, err)
	}
}
