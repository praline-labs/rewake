package wrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/channel"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/sessionstate"
	"github.com/praline-labs/rewake/internal/state"
)

// The launch in order (docs/mail-bridge-launch.md#the-launch-in-order): the
// name check runs before the claim, so a refusal leaves nothing behind; a
// launch without the tool says why; one with it hands the plan its server and
// shows the channel in the session state.

// toolHarness is the fake harness as one that carries the tool: its check
// answers as scripted and its launch keeps what it was handed.
type toolHarness struct {
	*fakeHarness
	decision harness.ToolDecision
	refusal  error
	checked  int
	launched *harness.ToolServer
	called   bool
	gates    harness.Gates
}

func (h *toolHarness) ID() string { return "claude" }

func (h *toolHarness) CheckMailTool(request harness.ToolCheckRequest) (harness.ToolDecision, error) {
	h.checked++
	h.gates = request.Gates
	return h.decision, h.refusal
}

func (h *toolHarness) Launch(request harness.LaunchRequest) (harness.LaunchPlan, error) {
	h.called = true
	h.launched = request.MailTool
	return h.fakeHarness.Launch(request)
}

func toolRequest(dir string, h *toolHarness) Request {
	return Request{Harness: h, Dir: dir, Name: "api", MailTool: &MailTool{}}
}

func TestARefusedNameLeavesNothingBehind(t *testing.T) {
	dir := stateDir(t)
	h := &toolHarness{fakeHarness: &fakeHarness{script: "exit 0"}, refusal: &harness.NameTakenError{Where: harness.Where{Scope: harness.ScopeUser}}}
	_, err := Run(context.Background(), toolRequest(dir, h))
	var taken *harness.NameTakenError
	if !errors.As(err, &taken) || h.checked != 1 || h.called {
		t.Fatalf("got %v, checked %d, launched %v", err, h.checked, h.called)
	}
	sessions, _ := registry.ListReadOnly(dir)
	if len(sessions) != 0 {
		t.Fatalf("a refused launch published %d sessions", len(sessions))
	}
	if entries, _ := os.ReadDir(state.InboxPath(dir, "api-claude")); len(entries) != 0 {
		t.Fatal("a refused launch made a mailbox")
	}
}

func TestNoMailToolAsksNothing(t *testing.T) {
	dir := stateDir(t)
	h := &toolHarness{fakeHarness: &fakeHarness{script: "exit 0"}, decision: harness.ToolDecision{Inject: true}}
	request := toolRequest(dir, h)
	request.NoMailTool = true
	if _, err := Run(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if h.checked != 0 || h.launched != nil {
		t.Fatalf("checked %d, launched with %+v", h.checked, h.launched)
	}
}

func TestTheChannelShowsWhatTheLaunchDecided(t *testing.T) {
	for _, tc := range []struct {
		name     string
		decision harness.ToolDecision
		assumed  []string
		injected bool
	}{
		{"no tool", harness.ToolDecision{Reason: "gate G7: not settled"}, nil, false},
		{"the tool", harness.ToolDecision{Inject: true}, []string{harness.GateG7}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := shortStateDir(t)
			h := &toolHarness{fakeHarness: &fakeHarness{script: "sleep 2"}, decision: tc.decision}
			request := toolRequest(dir, h)
			request.AssumedGates = tc.assumed
			done := make(chan error, 1)
			go func() { _, err := Run(context.Background(), request); done <- err }()
			record, session := waitForChannel(t, dir)
			if (record.Tool != channel.ToolNone) != tc.injected || tc.injected == (record.Reason != "") {
				t.Fatalf("channel %+v", record)
			}
			if !equalStrings(session.AssumedGates, tc.assumed) || !equalStrings(h.gates.Assumed(), tc.assumed) {
				t.Fatalf("assumed gates %v, checked under %v", session.AssumedGates, h.gates.Assumed())
			}
			if (h.launched != nil) != tc.injected {
				t.Fatalf("the plan was handed %+v", h.launched)
			}
			var config string
			if h.launched != nil {
				config = h.launched.ConfigFile
				if filepath.Dir(config) != filepath.Dir(registry.SocketFor(dir, session.Name, session.Epoch())) {
					t.Fatalf("the server file %s is not beside the run's sockets", config)
				}
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if config != "" {
				if _, err := os.Stat(config); err == nil {
					t.Fatal("the server file outlived the run")
				}
			}
		})
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

// waitForChannel waits for the run's channel record in its session state.
func waitForChannel(t *testing.T, dir string) (channel.Record, registry.Session) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if session, err := registry.Load(dir, "api-claude"); err == nil {
			if snapshot := sessionstate.Load(dir, session.Name, session.Epoch()); snapshot.Channel != nil {
				return *snapshot.Channel, session
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no channel record in the session state")
	return channel.Record{}, registry.Session{}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

// Until G8 closes no call's limits are proven, so no result acknowledges a
// read; taken as closed for a live check, they are.
func TestTheLimitsAreProvenOnlyWhenG8Closed(t *testing.T) {
	request := Request{Harness: &toolHarness{fakeHarness: &fakeHarness{}}, MailTool: &MailTool{}}
	tool := &mailTool{}
	for _, assumed := range [][]string{nil, {harness.GateG7}, {harness.GateG8}} {
		gates := harness.ResolveGates("claude", "", assumed)
		cfg := tool.config(request, "api", "e1", transports["claude"], nil, gates)
		if want := gates.Assumed() != nil && equalStrings(assumed, []string{harness.GateG8}); cfg.LimitsProven != want {
			t.Fatalf("assumed %v: limits proven %v", assumed, cfg.LimitsProven)
		}
	}
}
