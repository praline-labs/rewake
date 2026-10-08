//go:build rewakefixture

package toolrig

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/bridge/endpoint"
	"github.com/praline-labs/rewake/internal/cli"
	"github.com/praline-labs/rewake/internal/state"
)

// The transport's exchange around the one tool, rebuilt from
// bridge/server/mcp_test.go: what the old rig's server spoke over its stdio,
// the endpoint's transport role speaks over one connection per request. The
// protocol's own ping, cancellation and id rules have no counterpart here;
// what they guarded does — the tools offered, what comes in bounded, calls at
// once, a call nobody waits for, and the calls taken when the wrapper closes.

// The harness's process is offered the tools the CLI builds from its command
// table, and every name it was offered runs.
func TestTheTransportIsOfferedTheCLIsTools(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.start()
	offered, err := r.ask(command{Op: "offered"})
	if err != nil {
		t.Fatal(err)
	}
	want := cli.ToolDescriptors()
	if offered.Raw != bridge.DescriptorsDigest(want) || len(offered.Texts) != len(want) {
		t.Fatalf("offered %v (%s), the CLI builds %d tools", offered.Texts, offered.Raw, len(want))
	}
	for i, tool := range want {
		if offered.Texts[i] != tool.Name {
			t.Fatalf("tool %d offered as %q, built as %q", i, offered.Texts[i], tool.Name)
		}
	}
	r.nextTurn()
	if who := r.call("whoami"); who.ended || who.result.IsError || !strings.Contains(who.result.text(), "api") {
		t.Fatalf("whoami: %+v", who.result)
	}
}

// What comes in is bounded: a request past its bound, or one that is not a
// call, is refused and runs nothing, and the endpoint goes on.
func TestTheEndpointBoundsWhatComesIn(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.start()
	turn := r.nextTurn()
	long := `{"id":1,"op":"call","call":{"tool":"send","callId":"long","conversation":"` + thread + `","turn":"` + turn + `","arguments":{"text":"` + strings.Repeat("x", bridge.MaxWordsBytes+32<<10) + `"}}}`
	for _, raw := range []struct{ line, says string }{
		{long, "longer than a request can be"},
		{"not json", "asks only to run a call"},
		{`{"id":2,"op":"confirm"}`, "asks only to run a call"},
		{`{"id":"` + strings.Repeat("i", 200) + `","op":"call"}`, "asks only to run a call"},
	} {
		answer, ended := r.requestWith(command{Op: "request", Raw: raw.line})
		if ended || !strings.Contains(answer.Raw, raw.says) || !strings.Contains(answer.Raw, "nothing ran") {
			t.Fatalf("%.60s: %+v", raw.line, answer)
		}
	}
	if headsUps(r) != 0 || report(r) != "" {
		t.Fatal("a request past its bound ran")
	}
	if who := r.call("whoami"); who.ended || who.result.IsError {
		t.Fatalf("the endpoint did not go on: %+v", who.result)
	}
}

// Four calls run at once; a fifth is refused as busy, nothing run.
func TestAFifthCallIsBusy(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.start()
	turn := r.nextTurn()
	// Calls the harness never reported wait out the observation's two
	// seconds, holding their slots.
	var waiting sync.WaitGroup
	tool, arguments := toolOf([]string{"whoami"})
	for i := range 4 {
		waiting.Add(1)
		go func() {
			defer waiting.Done()
			_, _ = r.ask(command{Op: "request", Turn: turn, Call: "held-" + string(rune('a'+i)), Tool: tool, Arguments: arguments})
		}()
	}
	time.Sleep(300 * time.Millisecond)
	started := time.Now()
	r.observe(turn, "fifth", []string{"whoami"})
	fifth, _ := r.request(turn, "fifth", []string{"whoami"})
	if !fifth.IsError || !strings.Contains(fifth.text(), "four tool calls are running") || time.Since(started) > time.Second {
		t.Fatalf("a fifth call: %+v", fifth)
	}
	waiting.Wait()
}

// A call whose connection the harness dropped is answered to nobody, reads
// nothing, gives its slot back, and the endpoint answers the next; the
// letter shows to the next call.
func TestACallNobodyWaitsForIsNotAnswered(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.start()
	id := r.letter("the letter of a dropped call")
	turn := r.nextTurn()
	r.observe(turn, "dropped", []string{"inbox"})
	tool, arguments := toolOf([]string{"inbox"})
	if answer, ended := r.requestWith(command{Op: "request", Turn: turn, Call: "dropped", Tool: tool, Arguments: arguments, Unread: true}); ended || answer.Error != "" {
		t.Fatalf("the dropped request: %+v", answer)
	}
	time.Sleep(time.Second)
	if !r.unread(id) {
		t.Fatal("a call nobody read read the letter")
	}
	next := r.call("inbox")
	if next.ended || !strings.Contains(next.result.text(), "the letter of a dropped call") {
		t.Fatalf("the next call: %+v", next.result)
	}
	if err := r.complete(next, true); err != nil || r.unread(id) {
		t.Fatalf("the next call's read: %v, unread %v", err, r.unread(id))
	}
}

// As the wrapper closes, the endpoint finishes the calls it took and answers
// them, and Close returns once it did.
func TestTheEndpointFinishesItsCallsAsItCloses(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	h := newHold(t)
	t.Cleanup(h.free)
	r.fault = h.specAt("child", "confirmed")
	r.start()
	turn := r.nextTurn()
	r.observe(turn, "last", []string{"whoami"})
	answered := make(chan callResult, 1)
	go func() {
		result, _ := r.request(turn, "last", []string{"whoami"})
		answered <- result
	}()
	h.reached(t)
	closed := make(chan struct{})
	go func() { r.endpoint.Close(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("Close returned while a call it took was running")
	case <-time.After(300 * time.Millisecond):
	}
	h.release(t)
	select {
	case result := <-answered:
		if result.IsError || !strings.Contains(result.text(), "api") {
			t.Fatalf("the last call: %+v", result)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the last call was not answered")
	}
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close outlived the call it finished")
	}
}

// T2: a call runs only for the harness's own process, checked on each
// request: the same request from another process — here the test, which plays
// the wrapper and started the program — runs nothing.
func TestOnlyTheHarnesssOwnProcessIsServed(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.start()
	turn := r.nextTurn()
	r.observe(turn, "foreign", []string{"send", "--notify", "--wait", "0", "web", faultHeadsUp})
	tool, arguments := toolOf([]string{"send", "--notify", "--wait", "0", "web", faultHeadsUp})
	call := endpoint.ToolCall{Tool: tool, Arguments: arguments, CallID: "foreign", Conversation: thread, Turn: turn, TurnsNeverReused: true}
	answer, err := endpoint.CallTool(state.ContextPath(r.dir, "api", r.self.Epoch()), call, 5*time.Second)
	if err == nil || !strings.Contains(err.Error(), "not the harness's own process") {
		t.Fatalf("a request from another process: %+v %v", answer, err)
	}
	if headsUps(r) != 0 {
		t.Fatal("a request from another process ran")
	}
	// The program's own request of the same call still runs it.
	if result, ended := r.request(turn, "foreign", []string{"send", "--notify", "--wait", "0", "web", faultHeadsUp}); ended || !strings.Contains(result.text(), "pending for web") || headsUps(r) != 1 {
		t.Fatalf("the harness's own request: %+v", result)
	}
}
