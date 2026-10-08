package endpoint

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/state"
)

// The transport's side of a call (transport.go): the harness's own process
// asks, the endpoint runs the call in a child of its own image and answers.

var inboxTool = bridge.ToolDescriptor{Name: "inbox", Summary: "read", Params: []bridge.ToolParam{
	{Name: "peek", Kind: bridge.ParamSwitch, Description: "preview"},
	{Name: "text", Kind: bridge.ParamWord, Description: "a word"},
}}

// transportEndpoint serves a run whose transport is this test process, and
// whose calls run in this test binary as their child.
func transportEndpoint(t *testing.T, change ...func(*Config)) (*Endpoint, string) {
	t.Helper()
	served, path := testEndpoint(t, testTransport, append([]func(*Config){func(c *Config) {
		c.Tools = []bridge.ToolDescriptor{inboxTool}
		c.Check = func(words []string) ([]string, string) { return words, "" }
		c.Executable = os.Args[0]
	}}, change...)...)
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	served.SetTransport(os.Getpid(), start)
	served.SetChildEnv(func(name string) string {
		if name == state.DirEnv {
			return served.cfg.Dir
		}
		return ""
	})
	return served, path
}

func inboxCall(call string, arguments string) ToolCall {
	return ToolCall{Tool: "inbox", Arguments: json.RawMessage(arguments), CallID: call, Conversation: "th", Turn: "t1", TurnsNeverReused: true}
}

// A call the harness reported runs once, in a child that confirms its own
// ticket, and its answer is the child's.
func TestATransportsCallRunsInAChildOfTheWrapper(t *testing.T) {
	served, path := transportEndpoint(t)
	openTurn(served, "th", "t1")
	seenCall(served, "th", "t1", "c1", []string{"inbox", "--peek"})
	answer, err := CallTool(path, inboxCall("c1", `{"peek":true}`), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if answer.IsError || len(answer.Texts) != 1 || answer.Texts[0] != "ran: inbox --peek\n" {
		t.Fatalf("the answer: %+v", answer)
	}
	served.calls.mu.Lock()
	entry := served.calls.byCall["c1"]
	served.calls.mu.Unlock()
	if entry == nil || entry.issued == nil || !entry.issued.used || !entry.issued.ticket.TurnsNeverReused {
		t.Fatalf("the call's ticket: %+v", entry)
	}
	// The same call asked again runs nothing.
	again, err := CallTool(path, inboxCall("c1", `{"peek":true}`), 10*time.Second)
	if err != nil || !again.IsError || !strings.Contains(again.Texts[0], "already has its ticket") {
		t.Fatalf("the same call again: %+v %v", again, err)
	}
}

// Only the transport's own process is served: another process, a command
// running below the transport among them, is refused before anything runs.
func TestOnlyTheTransportsOwnProcessIsServed(t *testing.T) {
	served, path := transportEndpoint(t)
	openTurn(served, "th", "t1")
	helper := exec.Command(os.Args[0])
	helper.Env = append(os.Environ(), helperTransport+"="+path)
	out, err := helper.Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "not the harness's own process") {
		t.Fatalf("a process below the transport: %s", out)
	}
	served.SetTransport(0, 0)
	if _, err := CallTool(path, inboxCall("c2", `{}`), 5*time.Second); err == nil || !strings.Contains(err.Error(), "no tool transport") {
		t.Fatalf("a run without a transport: %v", err)
	}
	served.calls.mu.Lock()
	defer served.calls.mu.Unlock()
	if len(served.calls.spent) != 0 {
		t.Fatalf("a refused process got %d tickets", len(served.calls.spent))
	}
}

// The peer is checked again before the answer: a transport replaced while its
// call ran gets no answer, whatever the call did.
func TestAnAnswerGoesOnlyToTheTransportThatAsked(t *testing.T) {
	var served *Endpoint
	served, path := transportEndpoint(t, func(c *Config) {
		c.Check = func(words []string) ([]string, string) {
			served.SetTransport(1, 1)
			return words, ""
		}
	})
	openTurn(served, "th", "t1")
	seenCall(served, "th", "t1", "c3", []string{"inbox"})
	if answer, err := CallTool(path, inboxCall("c3", `{}`), 10*time.Second); err == nil {
		t.Fatalf("a replaced transport was answered: %+v", answer)
	}
}

// A request is bounded before it is parsed: one past the bound runs nothing
// and is not read as JSON at all.
func TestATransportsRequestIsBoundedBeforeItIsParsed(t *testing.T) {
	checked := false
	served, path := transportEndpoint(t, func(c *Config) {
		c.Check = func(words []string) ([]string, string) { checked = true; return words, "" }
	})
	openTurn(served, "th", "t1")
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	reader := bufio.NewReader(conn)
	_, _ = conn.Write([]byte(`{"role":"transport"}` + "\n"))
	if line, err := reader.ReadString('\n'); err != nil || strings.Contains(line, "error") {
		t.Fatalf("the hello: %q %v", line, err)
	}
	// Not even JSON: past the bound, nothing is parsed.
	_, _ = conn.Write([]byte("{" + strings.Repeat("x", maxCall+1) + "\n"))
	line, err := reader.ReadString('\n')
	if err != nil || !strings.Contains(line, "longer than a request can be") {
		t.Fatalf("an oversized request: %q %v", line, err)
	}
	if checked {
		t.Fatal("an oversized request was checked")
	}
}

// What the call cannot be is refused before a ticket: a tool the run does not
// offer, arguments the tool does not take, words the CLI refuses.
func TestACallTheToolsDoNotOfferRunsNothing(t *testing.T) {
	served, path := transportEndpoint(t, func(c *Config) {
		c.Check = func(words []string) ([]string, string) {
			if len(words) > 1 {
				return nil, "the CLI's own refusal\n"
			}
			return words, ""
		}
	})
	openTurn(served, "th", "t1")
	for _, refused := range []struct {
		call ToolCall
		says string
	}{
		{ToolCall{Tool: "withdraw", CallID: "r1", Conversation: "th", Turn: "t1"}, "no tool named"},
		{inboxCall("r2", `{"peek":"yes"}`), "true or false"},
		{inboxCall("r3", `{"peek":true}`), "the CLI's own refusal"},
	} {
		answer, err := CallTool(path, refused.call, 5*time.Second)
		if err != nil || !answer.IsError || !strings.Contains(strings.Join(answer.Texts, ""), refused.says) {
			t.Errorf("%s: %+v %v", refused.call.CallID, answer, err)
		}
	}
	served.calls.mu.Lock()
	defer served.calls.mu.Unlock()
	if len(served.calls.spent) != 0 {
		t.Fatalf("refused calls got %d tickets", len(served.calls.spent))
	}
}

// An answer past one result's bound is replaced whole: the model gets the
// endpoint's line, never a cut answer.
func TestAnAnswerPastItsBoundIsReplacedWhole(t *testing.T) {
	served, path := transportEndpoint(t)
	openTurn(served, "th", "t1")
	long := strings.Repeat("y", bridge.ResultCap)
	seenCall(served, "th", "t1", "big", []string{"inbox", "--", long})
	answer, err := CallTool(path, inboxCall("big", `{"text":"`+long+`"}`), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !answer.IsError || len(answer.Texts) != 1 || answer.Texts[0] != overBound {
		t.Fatalf("an answer past its bound: %+v", answer)
	}
	encoded, _ := json.Marshal(answer)
	if len(encoded) > bridge.ResultCap {
		t.Fatalf("the replacement is %d bytes", len(encoded))
	}
}
