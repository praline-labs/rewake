package wrap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/bridge/endpoint"
	"github.com/praline-labs/rewake/internal/host/launch"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/state"
)

// MailTool is what the CLI lends the wrapper for the mail tool's endpoint
// (docs/mail-bridge.md): its own check of a call's words, and its
// acknowledgment of a read. The wrapper imports neither, so the CLI stays the
// one implementation of the mail.
type MailTool struct {
	Words       func([]string) ([]string, error)
	Acknowledge func(dir, name, epoch, token string, evidence bridge.Exposure, gate bridge.EndGate) error
	// Check is the CLI's refusal of a transport's call by its words, and
	// Tools the commands a transport registers as tools.
	Check func([]string) ([]string, string)
	Tools []bridge.ToolDescriptor
}

// mailTool is a run's endpoint, or nothing where the run has none: a harness
// that carries no tool calls of its own, or an endpoint that could not start,
// which costs the tool and never the session.
type mailTool struct {
	endpoint   *endpoint.Endpoint
	clock      *inbox.ReadClock
	capability string
	// keeper keeps the run's channel record. Nothing makes one yet: S10
	// defines the neutral channel inputs and S11 wires the keeper to the
	// live set and the shell's evidence; until then every use of it is a
	// no-op and the session shows its mail channel as unknown.
	keeper *channelKeeper
	// tools and path are what a backend that carries the calls itself is
	// offered: the tools to register, and the endpoint to ask.
	tools []bridge.ToolDescriptor
	path  string
}

// config is the run's endpoint configuration.
func (t *mailTool) config(request Request, name, epoch, transport string, clock *inbox.ReadClock) endpoint.Config {
	span, refusal := endpoint.DeadlineFor("", os.Getenv)
	return endpoint.Config{
		Dir: request.Dir, Name: name, Epoch: epoch, Transport: transport,
		Capability: t.capability, Span: span, Refusal: refusal,
		Words: request.MailTool.Words, Acknowledge: request.MailTool.Acknowledge,
		Check: request.MailTool.Check, Tools: request.MailTool.Tools,
		Gate:    endpoint.NewGate(clock.Snapshot),
		Channel: t.keeper.tell,
	}
}

// startMailTool opens the run's context endpoint with a capability of its
// own, for a harness whose process carries the tool's calls. Before the
// plan: the plan offers the endpoint to the backend, and its failing leaves
// the tool out.
func startMailTool(request Request, name, epoch string) *mailTool {
	tool := &mailTool{}
	transport := request.Harness.Offers().ToolTransport
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
	tool.capability = hex.EncodeToString(secret)
	served, err := endpoint.Listen(state.ContextPath(request.Dir, name, epoch), tool.config(request, name, epoch, transport, clock))
	if err != nil {
		clock.Close()
		_, _ = fmt.Fprintln(os.Stderr, "rewake: the mail tool is not served: "+err.Error())
		return tool
	}
	tool.endpoint, tool.clock = served, clock
	tool.tools, tool.path = request.MailTool.Tools, state.ContextPath(request.Dir, name, epoch)
	return tool
}

// attach hands the endpoint what only the plan knows: the run's word on
// whether the tool may read, and the tools for the run to register.
func (t *mailTool) attach(plan adapter.LaunchPlan) {
	if t.endpoint == nil {
		return
	}
	transport, ok := plan.Service.(adapter.ToolTransport[bridge.ToolDescriptor])
	if !ok {
		return
	}
	t.endpoint.SetReadsOff(transport.ToolReadsOff)
	if len(t.tools) > 0 {
		// A call's child gets the harness's launch values; the run names
		// the process that asks for it.
		env := map[string]string{}
		for _, entry := range plan.Env {
			if key, value, ok := strings.Cut(entry, "="); ok {
				env[key] = value
			}
		}
		t.endpoint.SetChildEnv(func(key string) string { return env[key] })
		transport.OfferTools(t.tools, t.path, t.endpoint.SetTransport)
	}
}

// handler is a completion handler whose ends are captured through the
// tool's gate, when the run has one.
func (t *mailTool) handler(capture func() *inbox.ReadBoundary, publish func(context.Context, adapter.TurnEnd) error) adapter.TurnHandler {
	handler := adapter.TurnHandler{Capture: capture, Publish: publish}
	if t.endpoint != nil {
		handler.EndCapture = t.endpoint.Gate().Capture
		handler.Tool = t.endpoint
	}
	return handler
}

// started names the processes the tool's calls may come from below: the
// harness, and the process of the run's own that speaks for it.
func (t *mailTool) started(harnessPID int, process adapter.ProcessService) {
	if t.endpoint == nil {
		return
	}
	roots := []int{harnessPID}
	if process != nil && process.ProcessID() > 0 {
		roots = append(roots, process.ProcessID())
	}
	t.endpoint.SetRoots(roots...)
}

func (t *mailTool) close() {
	if t.endpoint != nil {
		t.endpoint.Close()
		t.clock.Close()
	}
}

// readVersion is the launch's read of its harness's version, through the
// adapter's gate; its error refuses the launch before the claim. The program
// read is the one the launch will start.
func readVersion(request Request, cwd string) (string, error) {
	program := adapter.LaunchRequest{Command: request.Command}.Program(request.Harness.ID())
	return launch.Gate(request.Harness, program, launch.ProbeEnv(), cwd)
}
