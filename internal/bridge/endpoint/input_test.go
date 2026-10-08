package endpoint

import (
	"testing"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/receipt"
)

// Only a call whose ticket a child used, and whose binding names a record,
// reaches the acknowledgment — driven through the neutral input, as every
// harness's reports reach the endpoint.
func TestOnlyAUsedTicketWithItsBindingIsAcknowledged(t *testing.T) {
	var got acknowledged
	served, path := testEndpoint(t, testTransport, func(c *Config) { c.Acknowledge = got.record })
	served.TurnStarted("th", "t1")
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
