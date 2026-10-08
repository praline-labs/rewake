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
	"github.com/praline-labs/rewake/internal/harness"
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

// toolTransport is a harness whose own process carries the tool's calls to
// the endpoint, named by the transport its calls are bound under.
type toolTransport interface{ ToolTransport() string }

// toolOffer is a backend that registers the tools with its harness and names
// the process that asks the endpoint to run their calls.
type toolOffer interface {
	OfferTools(tools []bridge.ToolDescriptor, endpoint string, serve func(pid int, start uint64))
}

// mailTool is a run's endpoint, or nothing where the run has none: a harness
// that carries no tool calls of its own, or an endpoint that could not start,
// which costs the tool and never the session.
type mailTool struct {
	endpoint   *endpoint.Endpoint
	clock      *inbox.ReadClock
	capability string
	// observer is the plan's observer, set once the plan is made: the
	// endpoint asks it for the conversation a call belongs to.
	observer atomic.Pointer[harness.Observer]
	// keeper keeps the run's channel record. Nothing makes one yet: the
	// record's producer comes with the adapter capabilities, and until then
	// every use of it is a no-op and the session shows its mail channel as
	// unknown.
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
		Gate:         endpoint.NewGate(clock.Snapshot),
		Conversation: t.conversation,
		Channel:      t.keeper.tell,
	}
}

// startMailTool opens the run's context endpoint with a capability of its
// own, for a harness whose process carries the tool's calls. Before the
// plan: the plan offers the endpoint to the backend, and its failing leaves
// the tool out.
func startMailTool(request Request, name, epoch string) *mailTool {
	tool := &mailTool{}
	transport := ""
	if own, ok := request.Harness.(toolTransport); ok {
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
// may read.
func (t *mailTool) attach(plan harness.LaunchPlan) {
	if t.endpoint == nil {
		return
	}
	if plan.Observer != nil {
		observer := plan.Observer
		t.observer.Store(&observer)
	}
	if reads, ok := plan.Backend.(interface{ ToolReadsOff() string }); ok {
		t.endpoint.SetReadsOff(reads.ToolReadsOff)
	}
	if offer, ok := plan.Backend.(toolOffer); ok && len(t.tools) > 0 {
		// A call's child gets the harness's launch values; the backend names
		// the process that asks for it.
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
	if t.endpoint != nil {
		handler.EndCapture = t.endpoint.Gate().Capture
		handler.Tool = t.endpoint
	}
	return handler
}

// started names the processes the tool's calls may come from below: the
// harness, and a backend's own process.
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

// readVersion is the launch's read of its harness's version, for a harness
// that reads it on every launch; its error refuses the launch before the
// claim. The program read is the one the launch will start.
func readVersion(request Request, cwd string) (harness.Version, error) {
	reader, ok := request.Harness.(harness.LaunchVersionReader)
	if !ok {
		return harness.Version{}, nil
	}
	return reader.ReadLaunchVersion(harness.LaunchRequest{Command: request.Command}.Program(request.Harness.ID()), harness.ProbeEnv(), cwd)
}
