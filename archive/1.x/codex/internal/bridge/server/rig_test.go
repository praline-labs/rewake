package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/bridge/endpoint"
	"github.com/praline-labs/rewake/internal/cli"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/receipt"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/registry/registrytest"
	"github.com/praline-labs/rewake/internal/state"
)

// The server is tested as the harness meets it: a built binary of this
// module started with a pipe for stdin and stdout, a fake MCP client on the
// other end, and the run's endpoint served by the test, which plays the
// wrapper. The binary carries the fault seam (state/fault_build.go), so a
// test can end the server or its child at any step.

var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "bridge-server")
	if err != nil {
		panic(err)
	}
	binary = filepath.Join(dir, "rewake")
	build := exec.Command("go", "build", "-tags", "rewakefault", "-o", binary, "../../../cmd/rewake")
	build.Stdout, build.Stderr = os.Stderr, os.Stderr
	if err := build.Run(); err != nil {
		_ = os.RemoveAll(dir)
		fmt.Fprintln(os.Stderr, "the test binary does not build:", err)
		os.Exit(1)
	}
	state.Fault = wrapperFaults.apply
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// rig is one run: the session api with web as a live peer, its mailbox, the
// wrapper's endpoint and read clock, and the server the harness started.
type rig struct {
	t         *testing.T
	root, dir string
	self, web registry.Session
	transport string
	endpoint  *endpoint.Endpoint
	clock     *inbox.ReadClock
	server    *mcpClient
	// fault is the REWAKE_FAULT of the next server started.
	fault string
	// acknowledged receives the outcome of every acknowledgment.
	acknowledged chan error
	turn         int
	calls        int
	// observing is set while a completion is handled; blind says the
	// observer's reads may fail then.
	observing atomic.Bool
	blind     bool
	// completed are the calls whose result the harness already recorded:
	// the endpoint handles only the first.
	completed map[string]bool
}

const capability = "rig-capability"

func newRig(t *testing.T, transport string, change ...func(*endpoint.Config)) *rig {
	t.Helper()
	root, err := os.MkdirTemp("", "rig")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	_ = os.Chmod(root, 0o700)
	dir, err := state.RoomDir(root, "default")
	if err != nil {
		t.Fatal(err)
	}
	r := &rig{t: t, root: root, dir: dir, transport: transport, acknowledged: make(chan error, 64), completed: map[string]bool{}}
	r.self = publish(t, dir, "api", os.Getpid())
	sleeper := exec.Command("sleep", "120")
	if err := sleeper.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sleeper.Process.Kill(); _, _ = sleeper.Process.Wait() })
	r.web = publish(t, dir, "web", sleeper.Process.Pid)

	r.clock, err = inbox.OpenReadClock(context.Background(), dir, "api", r.self.Epoch())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.clock.Close)
	cfg := endpoint.Config{
		Dir: dir, Name: "api", Epoch: r.self.Epoch(), Transport: transport, Capability: capability,
		Span: 25 * time.Second, Words: cli.ToolWords, Gate: endpoint.NewGate(r.clock.Snapshot),
		Acknowledge: func(dir, name, epoch, token string, evidence bridge.Exposure, gate bridge.EndGate) error {
			err := cli.AcknowledgeRead(dir, name, epoch, token, evidence, gate)
			r.acknowledged <- err
			return err
		},
		Conversation: func() string { return "conversation" },
		// The rig's hooks send the default limits, as after G8 closed: the
		// acknowledgment is what these tests are about.
		LimitsProven: true,
		// The server is another binary than the test that plays its
		// wrapper; the build check is the endpoint's own test's.
		SameBuild: func(int) error { return nil },
	}
	for _, apply := range change {
		apply(&cfg)
	}
	r.endpoint, err = endpoint.Listen(state.ContextPath(dir, "api", r.self.Epoch()), cfg)
	if err != nil {
		t.Fatal(err)
	}
	r.endpoint.SetRoots(os.Getpid())
	t.Cleanup(r.endpoint.Close)
	return r
}

func publish(t *testing.T, dir, name string, pid int) registry.Session {
	t.Helper()
	start, err := proc.StartTime(pid)
	if err != nil {
		t.Fatal(err)
	}
	session := registry.Session{
		Name: name, Harness: "codex", ServicePID: pid, ServiceStart: start, Boot: registrytest.Boot(t),
		PIDNamespace: proc.Namespace(), CWD: dir, StartedAt: time.Now(),
		Socket: filepath.Join(dir, "sock", name+".sock"),
	}
	if err := registry.Publish(dir, session); err != nil {
		t.Fatal(err)
	}
	return session
}

// env is the server's environment as the launch sets it.
func (r *rig) env() []string {
	env := []string{
		state.DirEnv + "=" + r.root, state.RoomEnv + "=default",
		state.SessionEnv + "=api", state.EpochEnv + "=" + r.self.Epoch(),
		bridge.CapabilityEnv + "=" + capability,
		"PATH=" + os.Getenv("PATH"), "HOME=" + r.root, "LANG=C.UTF-8",
	}
	if r.fault != "" {
		env = append(env, "REWAKE_FAULT="+r.fault)
	}
	return env
}

// start starts a server and initializes it, replacing one that ended.
func (r *rig) start() *mcpClient {
	r.t.Helper()
	if r.server != nil {
		r.server.close()
	}
	r.server = startServer(r.t, r.env())
	r.server.initialize(r.t)
	return r.server
}

// letter leaves a task from web where an announced one waits in api's
// mailbox, as the run's wrapper leaves it once it told the harness.
func (r *rig) letter(text string) string {
	r.t.Helper()
	message := inbox.Message{ID: inbox.NewID(), From: "web", FromEpoch: r.web.Epoch(), To: "api", ToEpoch: r.self.Epoch(), Kind: inbox.Task, Text: text, CreatedAt: time.Now()}
	if err := inbox.Put(r.dir, message); err != nil {
		r.t.Fatal(err)
	}
	if err := inbox.PutLocal(r.dir, message); err != nil {
		r.t.Fatal(err)
	}
	return message.ID
}

// nextTurn starts a turn of the primary thread.
func (r *rig) nextTurn() string {
	r.turn++
	turn := "turn-" + strconv.Itoa(r.turn)
	if r.transport == bridge.CodexTransport {
		r.endpoint.CodexEvent(codexEvent("turn/started", turn, "", nil, nil))
	}
	return turn
}

// endTurn ends the current turn as the gateway does: the end captured
// through the gate, then the turn's record.
func (r *rig) endTurn() *inbox.ReadBoundary {
	boundary, _ := r.endpoint.Gate().Capture()
	turn := "turn-" + strconv.Itoa(r.turn)
	r.endpoint.CodexEvent(codexEvent("turn/completed", turn, "", nil, nil))
	return boundary
}

// toolCall is one call as the harness makes it, and what came of it.
type toolCall struct {
	id, turn string
	words    []string
	result   callResult
	ended    bool
}

// call makes one tool call of the current turn: the harness's own record of
// it, the request to the server, the answer.
func (r *rig) call(words ...string) toolCall {
	r.t.Helper()
	r.calls++
	turn := "turn-" + strconv.Itoa(r.turn)
	id := "call-" + strconv.Itoa(r.calls)
	r.observe(turn, id, words)
	result, ended := r.server.call(r.t, words, r.meta(turn, id))
	return toolCall{id: id, turn: turn, words: words, result: result, ended: ended}
}

func (r *rig) observe(turn, id string, words []string) {
	if r.transport == bridge.CodexTransport {
		r.endpoint.CodexEvent(codexEvent("item/started", turn, id, words, nil))
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"session_id": "conversation", "prompt_id": turn, "hook_event_name": "PreToolUse",
		"tool_name": "mcp__rewake__rewake", "tool_input": map[string]any{"words": words}, "tool_use_id": id,
		"mcp_server": map[string]any{"name": "rewake"},
	})
	if err := endpoint.Observe(state.ContextPath(r.dir, "api", r.self.Epoch()), payload, endpoint.HookLimits{}, time.Second); err != nil {
		r.t.Fatalf("the hook: %v", err)
	}
}

func (r *rig) meta(turn, id string) map[string]any {
	if r.transport == bridge.CodexTransport {
		return map[string]any{"callId": id, "threadId": "conversation", "x-codex-turn-metadata": map[string]any{"turn_id": turn}}
	}
	return map[string]any{"claudecode/toolUseId": id}
}

// complete reports the call's result as the harness recorded it, and waits
// for the acknowledgment it starts, if the call holds a record.
func (r *rig) complete(c toolCall, succeeded bool) error {
	r.t.Helper()
	// Whether the endpoint will acknowledge is read first: while the event
	// is handled, the reads of this process are the observer's, and a plan
	// may fail them (wrapperPlan.during).
	bound := r.acknowledges(c)
	r.observing.Store(true)
	defer r.observing.Store(false)
	if r.transport == bridge.CodexTransport {
		raw := codexEvent("item/completed", c.turn, c.id, c.words, c.result.Content)
		if !succeeded {
			raw = codexFailed(c.turn, c.id, c.words)
		}
		r.endpoint.CodexEvent(raw)
	}
	if !bound {
		return nil
	}
	wait := 10 * time.Second
	if r.blind {
		// An observer that cannot read the binding acknowledges nothing.
		wait = 2 * time.Second
	}
	select {
	case err := <-r.acknowledged:
		return err
	case <-time.After(wait):
		if !r.blind {
			r.t.Fatal("the acknowledgment did not run")
		}
		return errors.New("no acknowledgment ran")
	}
}

// acknowledges says whether the next completion of c starts an
// acknowledgment: its first, of a call that holds a record.
func (r *rig) acknowledges(c toolCall) bool {
	first := !r.completed[c.id]
	r.completed[c.id] = true
	_, err := receipt.Bound(r.dir, "api", r.self.Epoch(), bridge.CallKey(r.transport, "conversation", c.id))
	return first && err == nil
}

func codexEvent(method, turn, call string, words []string, content any) []byte {
	params := map[string]any{"threadId": "conversation"}
	if call == "" {
		params["turn"] = map[string]any{"id": turn}
	} else {
		status := "inProgress"
		var result any
		if method == "item/completed" {
			status, result = "completed", map[string]any{"content": content}
		}
		params["turnId"] = turn
		params["item"] = map[string]any{
			"type": "mcpToolCall", "id": call, "server": "rewake", "tool": "rewake", "status": status,
			"arguments": map[string]any{"words": words}, "error": nil, "result": result,
		}
	}
	encoded, _ := json.Marshal(map[string]any{"method": method, "params": params})
	return encoded
}

func codexFailed(turn, call string, words []string) []byte {
	encoded, _ := json.Marshal(map[string]any{"method": "item/completed", "params": map[string]any{
		"threadId": "conversation", "turnId": turn,
		"item": map[string]any{
			"type": "mcpToolCall", "id": call, "server": "rewake", "tool": "rewake", "status": "failed",
			"arguments": map[string]any{"words": words}, "error": map[string]any{"message": "the server went away"}, "result": nil,
		},
	}})
	return encoded
}
