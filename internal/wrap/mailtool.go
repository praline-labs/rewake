package wrap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/bridge/endpoint"
	"github.com/praline-labs/rewake/internal/channel"
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
	// Check is the CLI's refusal of a transport's call by its words, and
	// Tools the commands a transport registers as tools.
	Check func([]string) ([]string, string)
	Tools []bridge.ToolDescriptor
}

// toolTransport is a harness whose own process carries the tool's calls to
// the endpoint, named by the transport its calls are bound under.
type toolTransport interface{ ToolTransport() string }

// toolOffer is a backend that registers the tools with its harness and names
// the process that asks the endpoint to run their calls.
type toolOffer interface {
	OfferTools(tools []bridge.ToolDescriptor, endpoint string, serve func(pid int, start uint64))
}

// mailTool is a run's endpoint, or nothing where the run has none: a harness
// without the tool, or an endpoint that could not start, which costs the
// tool and never the session.
type mailTool struct {
	endpoint   *endpoint.Endpoint
	clock      *inbox.ReadClock
	capability string
	// observer is the plan's observer, set once the plan is made: the
	// endpoint asks it for the conversation a call belongs to.
	observer atomic.Pointer[harness.Observer]
	// keeper keeps the run's channel record, nil for a harness without a
	// tool server; it exists whether or not the endpoint started.
	keeper *channelKeeper
	// tools and path are what a backend that carries the calls itself is
	// offered: the tools to register, and the endpoint to ask.
	tools []bridge.ToolDescriptor
	path  string
}

// config is the run's endpoint configuration. What a gate leaves unknown
// is decided here: until G8 closes, no call's limits are proven, and no
// result acknowledges a read.
func (t *mailTool) config(request Request, name, epoch, transport string, clock *inbox.ReadClock, gates harness.Gates) endpoint.Config {
	timeout := ""
	if timed, ok := request.Harness.(harness.ToolTimeout); ok {
		timeout = timed.ToolTimeoutVariable()
	}
	span, refusal := endpoint.DeadlineFor(timeout, os.Getenv)
	bound, _ := gates.OutputBound()
	return endpoint.Config{
		Dir: request.Dir, Name: name, Epoch: epoch, Transport: transport,
		Capability: t.capability, Span: span, Refusal: refusal,
		Words: request.MailTool.Words, Acknowledge: request.MailTool.Acknowledge,
		Check: request.MailTool.Check, Tools: request.MailTool.Tools,
		Gate:         endpoint.NewGate(clock.Snapshot),
		Conversation: t.conversation,
		Channel:      t.keeper.tell,
		LimitsProven: !gates.Open(harness.GateG8),
		OutputBound:  bound,
	}
}

// transports names the tool's transport of each harness that has it.
var transports = map[string]string{"codex": bridge.CodexTransport, "claude": bridge.ClaudeTransport}

// startMailTool opens the run's context endpoint with a capability of its
// own. Before the plan: the plan names the endpoint and the capability to
// the tool's server, and either failing leaves the tool out
// (docs/mail-bridge-launch.md#the-launch-in-order).
func startMailTool(request Request, name, epoch string, gates harness.Gates) *mailTool {
	tool := &mailTool{}
	transport := transports[request.Harness.ID()]
	if transport != "" {
		// The channel record is kept for a server's transport only.
		tool.keeper = newChannelKeeper(request.Dir, name, epoch, request.Harness.ID())
	} else if own, ok := request.Harness.(toolTransport); ok && !request.NoMailTool {
		transport = own.ToolTransport()
	}
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
	served, err := endpoint.Listen(state.ContextPath(request.Dir, name, epoch), tool.config(request, name, epoch, transport, clock, gates))
	if err != nil {
		clock.Close()
		_, _ = fmt.Fprintln(os.Stderr, "rewake: the mail tool is not served: "+err.Error())
		return tool
	}
	tool.endpoint, tool.clock = served, clock
	tool.tools, tool.path = request.MailTool.Tools, state.ContextPath(request.Dir, name, epoch)
	return tool
}

// conversation is the plan observer's thread, "" before the plan or for an
// observer that tracks none.
func (t *mailTool) conversation() string {
	observer := t.observer.Load()
	if observer == nil {
		return ""
	}
	if threads, ok := (*observer).(interface{ Thread() (string, error) }); ok {
		thread, _ := threads.Thread()
		return thread
	}
	return ""
}

// attach hands the endpoint what only the plan knows: the observer that
// names a call's conversation, and the backend's word on whether the tool
// may read (codex: the injection check's output conditions).
func (t *mailTool) attach(plan harness.LaunchPlan) {
	if t.endpoint == nil {
		return
	}
	if plan.Observer != nil {
		observer := plan.Observer
		t.observer.Store(&observer)
	}
	if source, ok := plan.Observer.(harness.SessionStartSource); ok && t.keeper != nil {
		source.OnSessionStart(func() { t.keeper.tell(channel.Event{Kind: channel.SessionStarted}) })
	}
	if backend := plan.Backend; backend != nil {
		// The thread the gateway holds as primary, "" while a selection is
		// pending: a call of any other thread is refused before its wait.
		t.endpoint.SetPrimary(func() string {
			thread, err := backend.Thread()
			if err != nil {
				return ""
			}
			return thread
		})
	}
	if reads, ok := plan.Backend.(interface{ ToolReadsOff() string }); ok {
		t.endpoint.SetReadsOff(reads.ToolReadsOff)
	}
	if offer, ok := plan.Backend.(toolOffer); ok && len(t.tools) > 0 {
		// A call's child gets the harness's launch values, as a server's
		// children do; the backend names the process that asks for it.
		env := map[string]string{}
		for _, entry := range plan.Env {
			if key, value, ok := strings.Cut(entry, "="); ok {
				env[key] = value
			}
		}
		t.endpoint.SetChildEnv(func(key string) string { return env[key] })
		offer.OfferTools(t.tools, t.path, t.endpoint.SetTransport)
	}
}

// handler is a completion handler whose ends are captured through the
// tool's gate, when the run has one.
func (t *mailTool) handler(capture func() *inbox.ReadBoundary, publish func(context.Context, harness.Completion) error) harness.CompletionHandler {
	handler := harness.CompletionHandler{Capture: capture, Publish: publish}
	if t.keeper != nil {
		handler.Channel = t.keeper.tell
	}
	if t.endpoint != nil {
		handler.EndCapture = t.endpoint.Gate().Capture
		handler.ToolEvent = t.endpoint.CodexEvent
		handler.Tool = t.endpoint
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
