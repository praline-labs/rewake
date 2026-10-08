package wrap

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/bridge/endpoint"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/state"
)

// A harness whose own process carries the tool's calls: the wrapper starts the
// endpoint under the harness's transport, offers the backend the tools and the
// endpoint, and lets the backend name the process the endpoint serves.

// carrierHarness is the fake harness as one with a transport of its own.
type carrierHarness struct{ *fakeHarness }

func (carrierHarness) ToolTransport() string { return "test-transport" }

// offeringBackend records what the wrapper offered it.
type offeringBackend struct {
	tools    []bridge.ToolDescriptor
	endpoint string
	serve    func(int, uint64)
}

func (b *offeringBackend) OfferTools(tools []bridge.ToolDescriptor, path string, serve func(int, uint64)) {
	b.tools, b.endpoint, b.serve = tools, path, serve
}

func (*offeringBackend) Start(context.Context, harness.CompletionHandler, func(string)) error {
	return nil
}

func (*offeringBackend) Deliver(context.Context, inbox.Message) inbox.Result { return inbox.Result{} }
func (*offeringBackend) Thread() (string, error)                             { return "", nil }
func (*offeringBackend) Done() <-chan struct{}                               { return nil }
func (*offeringBackend) Close()                                              {}

var carriedTools = []bridge.ToolDescriptor{{Name: "whoami", Summary: "who"}}

func carrierTool(t *testing.T) *mailTool {
	t.Helper()
	dir := shortStateDir(t)
	request := Request{
		Harness: carrierHarness{&fakeHarness{}}, Dir: dir, Name: "api",
		MailTool: &MailTool{
			Words: func(w []string) ([]string, error) { return w, nil }, Tools: carriedTools,
			Check: func(w []string) ([]string, string) { return w, "" },
		},
	}
	tool := startMailTool(request, "api", "e1")
	t.Cleanup(tool.close)
	return tool
}

func TestAHarnessThatCarriesItsCallsIsOfferedTheTools(t *testing.T) {
	tool := carrierTool(t)
	if tool.endpoint == nil {
		t.Fatal("no endpoint for a harness that carries its calls")
	}
	if tool.keeper != nil {
		t.Fatal("a channel record kept for a transport with no server")
	}
	backend := &offeringBackend{}
	tool.attach(harness.LaunchPlan{Backend: backend, Env: []string{state.DirEnv + "=" + tool.path, "UNRELATED=1"}})
	if len(backend.tools) != 1 || backend.tools[0].Name != "whoami" || backend.endpoint != tool.path || backend.serve == nil {
		t.Fatalf("the offer: %+v", backend)
	}
	// Until the backend names a process the endpoint serves none; once it
	// names this one, a call reaches the tools.
	if _, err := endpoint.CallTool(tool.path, endpoint.ToolCall{Tool: "edit", CallID: "c1"}, 5*time.Second); err == nil || !strings.Contains(err.Error(), "no tool transport") {
		t.Fatalf("a call before the backend named its process: %v", err)
	}
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	backend.serve(os.Getpid(), start)
	answer, err := endpoint.CallTool(tool.path, endpoint.ToolCall{Tool: "edit", CallID: "c1"}, 5*time.Second)
	if err != nil || !answer.IsError || !strings.Contains(answer.Texts[0], "no tool named") {
		t.Fatalf("a call from the named process: %+v %v", answer, err)
	}
}

// shortStateDir is a state directory whose sockets fit a unix socket path,
// which a test's own temporary directory may not.
func shortStateDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "rw")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv(state.DirEnv, dir)
	resolved, err := state.Dir()
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}
