package endpoint

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/receipt"
)

// acknowledged collects what the endpoint handed the acknowledgment.
type acknowledged struct {
	mu     sync.Mutex
	tokens []string
	seen   []bridge.Exposure
}

func (a *acknowledged) record(_, _, _, token string, evidence bridge.Exposure, gate bridge.EndGate) error {
	if gate == nil {
		return errors.New("no gate")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tokens = append(a.tokens, token)
	a.seen = append(a.seen, evidence)
	return nil
}

func fixture(t *testing.T, name string) [][]byte {
	t.Helper()
	file, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	var lines [][]byte
	scanner := bufio.NewScanner(file)
	scanner.Buffer(nil, 1<<20)
	for scanner.Scan() {
		lines = append(lines, append([]byte(nil), scanner.Bytes()...))
	}
	return lines
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
	if err := receipt.Bind(cfg.Dir, cfg.Name, cfg.Epoch, bridge.CallKey(ticket.Transport, ticket.Conversation, ticket.CallID), token); err != nil {
		t.Fatal(err)
	}
}

func token(n int) string { return strings.Repeat("0", 23) + string(rune('a'+n)) }

// The recorded Codex stream reads as calls of the primary thread whose
// results are lists of text items: each answer is its first text.
func TestTheRecordedCodexStreamGivesEvidence(t *testing.T) {
	var got acknowledged
	served, path := testEndpoint(t, bridge.CodexTransport, func(c *Config) { c.Acknowledge = got.record })
	calls := 0
	for _, line := range fixture(t, "codex-stream.jsonl") {
		var m codexMessage
		if err := json.Unmarshal(line, &m); err != nil {
			t.Fatal(err)
		}
		if m.Method == "item/completed" {
			words, _ := wordsOf(m.Params.Item.Arguments)
			used(t, served, path, codexRequest(m.Params.Thread, m.Params.TurnID, m.Params.Item.ID, words), token(calls))
			calls++
		}
		served.CodexEvent(line)
	}
	served.Close()
	if calls == 0 || len(got.seen) != calls {
		t.Fatalf("%d acknowledgments for %d calls", len(got.seen), calls)
	}
	for i, evidence := range got.seen {
		if !evidence.Succeeded || !evidence.Direct || evidence.Shortened || len(evidence.Answer) == 0 || evidence.ResultBytes < len(evidence.Answer) || got.tokens[i] != token(i) {
			t.Fatalf("call %d: %+v under %s", i, evidence, got.tokens[i])
		}
	}
}

// The recorded hooks: a result the harness kept in a file arrives as a string
// and proves nothing; a list of texts is the answer.
func TestTheRecordedClaudeHooksGiveEvidence(t *testing.T) {
	var got acknowledged
	lines := fixture(t, "claude-hooks.jsonl")
	var first claudeHook
	_ = json.Unmarshal(lines[0], &first)
	served, path := testEndpoint(t, bridge.ClaudeTransport, func(c *Config) {
		c.Acknowledge = got.record
		c.Conversation = func() string { return first.Session }
	})
	calls := 0
	for _, line := range lines {
		var input claudeHook
		_ = json.Unmarshal(line, &input)
		served.hook(line)
		if input.Event == "PreToolUse" {
			words, _ := wordsOf(input.Input)
			used(t, served, path, TicketRequest{Transport: bridge.ClaudeTransport, CallID: input.UseID, Words: words, Digest: bridge.Digest(words)}, token(calls))
			calls++
		}
	}
	served.Close()
	if calls < 3 || len(got.seen) != calls {
		t.Fatalf("%d acknowledgments for %d calls", len(got.seen), calls)
	}
	for i, evidence := range got.seen {
		if want := i >= 2; evidence.Shortened == want || want != (len(evidence.Answer) > 0) || !evidence.Direct {
			t.Fatalf("call %d: %+v", i, evidence)
		}
	}
}

// Only a call whose ticket a child used, and whose binding names a record,
// reaches the acknowledgment.
func TestOnlyAUsedTicketWithItsBindingIsAcknowledged(t *testing.T) {
	var got acknowledged
	served, path := testEndpoint(t, bridge.CodexTransport, func(c *Config) { c.Acknowledge = got.record })
	openTurn(served, "th", "t1")
	content := []map[string]string{{"type": "text", "text": "a letter"}}
	served.CodexEvent(codexEvent("item/started", "th", "t1", "never-asked", words, nil))
	served.CodexEvent(codexEvent("item/completed", "th", "t1", "never-asked", words, content))
	served.CodexEvent(codexEvent("item/started", "th", "t1", "not-used", words, nil))
	mustTicket(t, path, codexRequest("th", "t1", "not-used", words))
	// Bound all the same, so only the unused ticket keeps it out.
	cfg := served.cfg
	if err := receipt.Bind(cfg.Dir, cfg.Name, cfg.Epoch, bridge.CallKey(bridge.CodexTransport, "th", "not-used"), token(1)); err != nil {
		t.Fatal(err)
	}
	served.CodexEvent(codexEvent("item/completed", "th", "t1", "not-used", words, content))
	served.CodexEvent(codexEvent("item/started", "th", "t1", "unbound", words, nil))
	used(t, served, path, codexRequest("th", "t1", "unbound", words), "")
	served.CodexEvent(codexEvent("item/completed", "th", "t1", "unbound", words, content))
	served.CodexEvent(codexEvent("item/started", "th", "t1", "bound", words, nil))
	used(t, served, path, codexRequest("th", "t1", "bound", words), token(0))
	served.CodexEvent(codexEvent("item/completed", "th", "t1", "bound", words, content))
	served.Close()
	if len(got.seen) != 1 || got.seen[0].CallID != "bound" || string(got.seen[0].Answer) != "a letter" {
		t.Fatalf("acknowledged %+v", got.seen)
	}
}

// A Codex result that failed, or that the harness marked an error, proves no
// success.
func TestAFailedCodexResultIsNoSuccess(t *testing.T) {
	var got acknowledged
	served, path := testEndpoint(t, bridge.CodexTransport, func(c *Config) { c.Acknowledge = got.record })
	openTurn(served, "th", "t1")
	failures := []string{
		`"status":"failed","error":{"message":"x"},"result":{"content":[{"type":"text","text":"a"}]}`,
		`"status":"completed","error":null,"result":{"content":[{"type":"text","text":"a"}],"isError":true}`,
		`"status":"completed","error":null,"result":null`,
	}
	for i, shape := range failures {
		call := "failed-" + string(rune('a'+i))
		served.CodexEvent(codexEvent("item/started", "th", "t1", call, words, nil))
		used(t, served, path, codexRequest("th", "t1", call, words), token(i))
		served.CodexEvent([]byte(`{"method":"item/completed","params":{"threadId":"th","turnId":"t1","item":{"type":"mcpToolCall","id":"` + call + `","server":"rewake","tool":"rewake","arguments":{"words":["inbox"]},` + shape + `}}}`))
	}
	served.Close()
	if len(got.seen) != len(failures) {
		t.Fatalf("%d acknowledgments", len(got.seen))
	}
	for _, evidence := range got.seen {
		if evidence.Succeeded {
			t.Fatalf("a failed result counted as a success: %+v", evidence)
		}
	}
}

// The exposure of each result shape.
func TestTheExposureOfEachResultShape(t *testing.T) {
	cases := []struct {
		content   string
		shortened bool
		answer    string
	}{
		{`[{"type":"text","text":"first"},{"type":"text","text":"second"}]`, false, "first"},
		{`"Error: result exceeds maximum allowed tokens. Output has been saved to a file"`, true, ""},
		{`[{"type":"image","data":"AAAA"}]`, true, ""},
		{`[{"type":"text","text":"first"},{"type":"image","data":"AAAA"}]`, true, ""},
		{`[]`, true, ""},
		{``, true, ""},
	}
	for _, test := range cases {
		evidence := exposureOf("c", json.RawMessage(test.content), true, true)
		if evidence.Shortened != test.shortened || string(evidence.Answer) != test.answer {
			t.Fatalf("%s: %+v", test.content, evidence)
		}
		if !test.shortened && evidence.ResultBytes != bridge.EncodedSize("first", "second") {
			t.Fatalf("%s: %d bytes", test.content, evidence.ResultBytes)
		}
	}
}

// A Claude Code call's turn is its prompt, renamed after an end noted once
// the prompt was first heard, and unknown when the journals are unreadable.
// A nested agent's call and a call in another conversation are refused.
func TestAClaudeCallsTurn(t *testing.T) {
	served, path := testEndpoint(t, bridge.ClaudeTransport, func(c *Config) { c.Conversation = func() string { return "conv" } })
	pre := func(prompt, call string, extra string) TicketRequest {
		served.hook([]byte(`{"session_id":"conv","prompt_id":"` + prompt + `","hook_event_name":"PreToolUse","tool_name":"mcp__rewake__rewake","tool_input":{"words":["inbox"]},"tool_use_id":"` + call + `","mcp_server":{"name":"rewake"}` + extra + `}`))
		return TicketRequest{Transport: bridge.ClaudeTransport, CallID: call, Words: words, Digest: bridge.Digest(words)}
	}
	if ticket := mustTicket(t, path, pre("p1", "c1", "")); ticket.Turn != "p1" || ticket.Conversation != "conv" {
		t.Fatalf("the first call: %+v", ticket)
	}
	journal := func(id string, ended int64) {
		if err := inbox.WriteJournal(served.cfg.Dir, "api", id, inbox.TurnJournal{Epoch: "e1", Op: "end", Ended: ended}); err != nil {
			t.Fatal(err)
		}
	}
	first := served.calls.firstSeen["p1"]
	journal("early", first-1)
	if ticket := mustTicket(t, path, pre("p1", "c2", "")); ticket.Turn != "p1" {
		t.Fatalf("an end before the prompt renamed it: %+v", ticket)
	}
	journal("later", first+5)
	if ticket := mustTicket(t, path, pre("p1", "c3", "")); ticket.Turn != "p1@"+itoa(first+5) {
		t.Fatalf("an end after the prompt did not rename it: %+v", ticket)
	}
	_, err := ticketFor(t, path, pre("p1", "c4", `,"agent_id":"a1","agent_type":"general"`))
	refusedWith(t, err, "a nested agent")
	served.hook([]byte(`{"session_id":"other","prompt_id":"p1","hook_event_name":"PreToolUse","tool_name":"mcp__rewake__rewake","tool_input":{"words":["inbox"]},"tool_use_id":"c5"}`))
	_, err = ticketFor(t, path, TicketRequest{Transport: bridge.ClaudeTransport, CallID: "c5", Words: words, Digest: bridge.Digest(words)})
	refusedWith(t, err, "another conversation")
	if err := os.WriteFile(filepath.Join(inbox.JournalPath(served.cfg.Dir, "api"), "broken"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = ticketFor(t, path, pre("p1", "c6", ""))
	if err == nil || !strings.Contains(err.Error(), "cannot be read") {
		t.Fatalf("unreadable journals: %v", err)
	}
	served.hook([]byte(`{"session_id":"conv","prompt_id":"p1","hook_event_name":"PreToolUse","tool_name":"mcp__other__rewake","tool_input":{"words":["inbox"]},"tool_use_id":"c7"}`))
	if served.calls.byCall["c7"] != nil {
		t.Fatal("another server's tool was observed")
	}
}

func itoa(n int64) string {
	encoded, _ := json.Marshal(n)
	return string(encoded)
}
