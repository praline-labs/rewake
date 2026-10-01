package wrap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/bridge/endpoint"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/state"
)

// MailTool is what the CLI lends the wrapper for the mail tool's endpoint
// (docs/mail-bridge-server.md): its own check of a call's words, and its
// acknowledgment of a read. The wrapper imports neither, so the CLI stays the
// one implementation of the mail.
type MailTool struct {
	Words       func([]string) ([]string, error)
	Acknowledge func(dir, name, epoch, token string, evidence bridge.Exposure, gate bridge.EndGate) error
}

// mailTool is a run's endpoint, or nothing where the run has none: a harness
// without the tool, or an endpoint that could not start, which costs the
// tool and never the session.
type mailTool struct {
	endpoint *endpoint.Endpoint
	clock    *inbox.ReadClock
}

// transports names the tool's transport of each harness that has it.
var transports = map[string]string{"codex": bridge.CodexTransport, "claude": bridge.ClaudeTransport}

// startMailTool opens the run's context endpoint with a capability of its
// own. Before the harness: its first hook may come with its first prompt.
func startMailTool(request Request, name, epoch string, plan harness.LaunchPlan) *mailTool {
	tool := &mailTool{}
	transport := transports[request.Harness.ID()]
	if request.MailTool == nil || transport == "" {
		return tool
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return tool
	}
	clock, err := inbox.OpenReadClock(context.Background(), request.Dir, name, epoch)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "rewake: the mail tool is not served: "+err.Error())
		return tool
	}
	span, refusal := endpoint.DeadlineFor(transport, os.Getenv)
	cfg := endpoint.Config{
		Dir: request.Dir, Name: name, Epoch: epoch, Transport: transport,
		Capability: hex.EncodeToString(secret), Span: span, Refusal: refusal,
		Words: request.MailTool.Words, Acknowledge: request.MailTool.Acknowledge,
		Gate: endpoint.NewGate(clock.Snapshot),
	}
	if threads, ok := plan.Observer.(interface{ Thread() (string, error) }); ok {
		cfg.Conversation = func() string { thread, _ := threads.Thread(); return thread }
	}
	served, err := endpoint.Listen(state.ContextPath(request.Dir, name, epoch), cfg)
	if err != nil {
		clock.Close()
		_, _ = fmt.Fprintln(os.Stderr, "rewake: the mail tool is not served: "+err.Error())
		return tool
	}
	tool.endpoint, tool.clock = served, clock
	return tool
}

// handler is a completion handler whose ends are captured through the
// tool's gate, when the run has one.
func (t *mailTool) handler(capture func() *inbox.ReadBoundary, publish func(context.Context, harness.Completion) error) harness.CompletionHandler {
	handler := harness.CompletionHandler{Capture: capture, Publish: publish}
	if t.endpoint != nil {
		handler.EndCapture = t.endpoint.Gate().Capture
		handler.ToolEvent = t.endpoint.CodexEvent
	}
	return handler
}

// started names the processes the tool's server may run below: the harness,
// and a backend's own server, which starts the tool servers on Codex.
func (t *mailTool) started(harnessPID int, backend harness.Backend) {
	if t.endpoint == nil {
		return
	}
	roots := []int{harnessPID}
	if owner, ok := backend.(harness.ProcessBackend); ok && owner.ProcessID() > 0 {
		roots = append(roots, owner.ProcessID())
	}
	t.endpoint.SetRoots(roots...)
}

func (t *mailTool) close() {
	if t.endpoint != nil {
		t.endpoint.Close()
		t.clock.Close()
	}
}
