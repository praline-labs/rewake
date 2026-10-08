//go:build rewakefixture

package fixture

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/harness"
)

// The tool transport's side in the adapter (tool.go): what the probe proves,
// which process the endpoint is told to serve, and what the program's reports
// of its calls become in the neutral input.

var programTools = []bridge.ToolDescriptor{
	{Name: "inbox", Summary: "read", Params: []bridge.ToolParam{{Name: "peek", Kind: bridge.ParamSwitch, Description: "preview"}}},
	{Name: "whoami", Summary: "who"},
}

// servedPeer records the process the backend last told the endpoint to serve.
type servedPeer struct {
	mu  sync.Mutex
	pid int
}

func (s *servedPeer) set(pid int, _ uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pid = pid
}

// toolPeers holds each started program's record, by its backend.
var toolPeers sync.Map

func peerOf(b *backend) int {
	served, ok := toolPeers.Load(b)
	if !ok {
		return 0
	}
	s := served.(*servedPeer)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pid
}

// toolInput records what the adapter reported, in order.
type toolInput struct {
	mu   sync.Mutex
	seen []string
}

func (r *toolInput) add(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, fmt.Sprintf(format, args...))
}

func (r *toolInput) events() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.seen)
}

func (r *toolInput) TurnStarted(conversation, turn string, at int64) {
	r.add("started %s %s %d", conversation, turn, at)
}

func (r *toolInput) TurnEnded(conversation, turn string) { r.add("ended %s %s", conversation, turn) }

func (r *toolInput) CallSeen(call harness.ObservedCall) {
	r.add("seen %s %s %s %q nested=%t", call.ID, call.Conversation, call.Turn, call.Words, call.Nested)
}

func (r *toolInput) CallResult(id string, result harness.ToolResult) {
	r.add("result %s %+v", id, result)
}

// The endpoint serves the program's own process once the probe proved the
// tools registered, and reads count only where the program can show results.
func TestALiveToolTransportNamesTheProgramToTheEndpoint(t *testing.T) {
	b, err := startProgram(t)
	if err != nil {
		t.Fatal(err)
	}
	if pid := peerOf(b); pid == 0 || pid != b.pid {
		t.Fatalf("the endpoint serves %d, not the program's %d", pid, b.pid)
	}
	if reason := b.ToolReadsOff(); reason != "" {
		t.Fatalf("a transport that shows its results turns reads off: %q", reason)
	}
	unproven, err := startProgram(t, switchProof+"=1")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(unproven.Live(), ToolTransport) || unproven.ToolReadsOff() == "" {
		t.Fatalf("a transport that cannot show its results: live %v, reads off %q", unproven.Live(), unproven.ToolReadsOff())
	}
}

// A program that registered other tools than those offered has no live
// transport, so the endpoint serves no process of it.
func TestOtherToolsRegisteredLeaveTheTransportOff(t *testing.T) {
	b, err := startProgram(t, switchOther+"=1")
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(b.Live(), ToolTransport) || peerOf(b) != 0 {
		t.Fatalf("live %v, serving %d", b.Live(), peerOf(b))
	}
}

// A call, its result and the turn around them reach the neutral input as the
// program reported them: the call's words from its arguments, the turn's
// start at the program's own time.
func TestTheProgramsReportsBecomeTheNeutralInput(t *testing.T) {
	heard := &toolInput{}
	publish := func(context.Context, harness.Completion) error { return nil }
	b, p := paired(t, harness.CompletionHandler{Tool: heard, Publish: publish}, Served...)
	b.tools.tools = programTools
	if answer := p.ask(t, Frame{Op: opTurnStarted, Turn: "t1", At: 4242}); !answer.OK {
		t.Fatalf("the turn's start: %+v", answer)
	}
	for _, call := range []Frame{
		{Op: opToolCall, Call: "c1", Turn: "t1", Tool: "inbox", Arguments: json.RawMessage(`{"peek":true}`)},
		{Op: opToolCall, Call: "c2", Turn: "t1", Tool: "inbox", Arguments: json.RawMessage(`{"peek":"no"}`)},
		{Op: opToolCall, Call: "c3", Turn: "t1", Tool: "withdraw"},
		{Op: opToolCall, Call: "c4", Turn: "t1", Tool: "whoami", Nested: true},
		{Op: opToolResult, Call: "c1", Texts: []string{"read: none\n"}},
		{Op: opToolResult, Call: "c4", IsError: true, Shortened: true, Nested: true},
	} {
		if answer := p.ask(t, call); !answer.OK {
			t.Fatalf("%s %s: %+v", call.Op, call.Call, answer)
		}
	}
	if answer := p.ask(t, Frame{Op: opTurnEnded, Turn: "t1", End: "t1/1", Outcome: OutcomeCompleted}); !answer.OK {
		t.Fatalf("the turn's end: %+v", answer)
	}
	want := []string{
		"started " + programThrd + " t1 4242",
		`seen c1 ` + programThrd + ` t1 ["inbox" "--peek"] nested=false`,
		`seen c2 ` + programThrd + ` t1 [] nested=false`,
		`seen c3 ` + programThrd + ` t1 [] nested=false`,
		`seen c4 ` + programThrd + ` t1 ["whoami"] nested=true`,
		`result c1 {Succeeded:true Direct:true Texts:[read: none
] Shortened:false}`,
		`result c4 {Succeeded:false Direct:false Texts:[] Shortened:true}`,
		"ended " + programThrd + " t1",
	}
	if got := heard.events(); !slices.Equal(got, want) {
		t.Fatalf("the neutral input:\n%q\nwant\n%q", got, want)
	}
}

// Without a live transport the program's reports of calls reach nothing.
func TestACallReportWithoutTheTransportIsRefused(t *testing.T) {
	heard := &toolInput{}
	b, p := paired(t, harness.CompletionHandler{Tool: heard}, Wake, TurnBoundary)
	b.tools.tools = programTools
	for _, frame := range []Frame{
		{Op: opToolCall, Call: "c1", Turn: "t1", Tool: "inbox"},
		{Op: opToolResult, Call: "c1", Texts: []string{"x"}},
	} {
		if answer := p.ask(t, frame); answer.OK {
			t.Fatalf("%s taken without a tool transport", frame.Op)
		}
	}
	if got := heard.events(); len(got) != 0 {
		t.Fatalf("reported without a tool transport: %q", got)
	}
}
