//go:build rewakefixture

package fixture

import (
	"slices"
	"sync"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/harness"
)

// The program's side of the mail tool (docs/v2/stage3-fixture.md#the-tool-transport).
// The wrapper offers the tools and its endpoint before the program starts; the
// probe hands both to the program, which registers the tools and answers their
// digest. Live, the program reports each call it observed and the result it
// handed the model, and asks the endpoint itself to run the call: the
// endpoint serves only the program's own process, and only while the
// capability is live, so a command the model runs below it is never served.

// Transport names the fixture's transport in a call's binding.
const Transport = "fixture-tool"

// The states a tool transport's probe answers: whether the program can show
// the result it handed the model, which a read needs before it counts.
const (
	ToolProven   = "proven"
	ToolUnproven = "unproven"
)

// toolOffer is what the wrapper offered and what the probe proved of it.
type toolOffer struct {
	tools    []bridge.ToolDescriptor
	endpoint string
	// serve names the process the endpoint takes calls from; 0 takes none.
	serve func(pid int, start uint64)

	// mu orders the calls to serve, so the last one says what is live now.
	mu       sync.Mutex
	unproven bool
}

// ToolTransport is the transport the fixture's calls are bound under.
func (fixtureHarness) ToolTransport() string { return Transport }

// OfferTools hands the backend the tools and the endpoint its program asks to
// run them, before Start; serve is how the endpoint learns which process asks.
func (b *backend) OfferTools(tools []bridge.ToolDescriptor, endpoint string, serve func(pid int, start uint64)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.tools.tools, b.tools.endpoint, b.tools.serve = tools, endpoint, serve
}

// toolProbe is the probe of the tool transport, or no frame without tools.
func (b *backend) toolProbe() Frame {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.tools.tools) == 0 || b.tools.serve == nil {
		return Frame{}
	}
	return Frame{Op: opProbe, Capability: ToolTransport, Tools: b.tools.tools, Endpoint: b.tools.endpoint}
}

// toolsRegistered says whether a tool transport's answer registered exactly
// the tools offered, and notes whether it can prove its results. Every other
// capability has nothing to register.
func (b *backend) toolsRegistered(capability string, answer Frame) bool {
	if capability != ToolTransport {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	names := make([]string, 0, len(b.tools.tools))
	for _, tool := range b.tools.tools {
		names = append(names, tool.Name)
	}
	if answer.Digest != bridge.DescriptorsDigest(b.tools.tools) || !slices.Equal(answer.Names, names) {
		return false
	}
	b.tools.unproven = answer.State == ToolUnproven
	return true
}

// toolsLive tells the endpoint which process it serves: the program while the
// tool transport is live on the connection held now, none otherwise.
func (b *backend) toolsLive() {
	b.tools.mu.Lock()
	defer b.tools.mu.Unlock()
	b.mu.Lock()
	serve, live := b.tools.serve, b.link != nil && !b.link.gone() && b.live[ToolTransport]
	b.mu.Unlock()
	if serve == nil {
		return
	}
	if live {
		serve(b.pid, b.start)
	} else {
		serve(0, 0)
	}
}

// ToolReadsOff says why a read through the tool cannot count: the program
// cannot show the result it handed the model (T6).
func (b *backend) ToolReadsOff() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.tools.unproven {
		return "the fixture cannot show the result its model was given"
	}
	return ""
}

// toolCall reports a call the program observed to the endpoint, which a
// request for its binding is matched against. Answered once reported, so the
// program's request after the answer finds it.
func (b *backend) toolCall(l *link, frame Frame) Frame {
	if !b.liveOn(l, ToolTransport) {
		return Frame{Error: "no live tool transport"}
	}
	if frame.Call == "" || frame.Turn == "" {
		return Frame{Error: "a tool call names itself and its turn"}
	}
	if !b.enter() {
		return Frame{Error: errClosing.Error()}
	}
	defer b.leave()
	if b.handler.Tool == nil {
		return Frame{Error: "the wrapper takes no tool calls"}
	}
	b.mu.Lock()
	tools, thread := b.tools.tools, b.thread
	b.mu.Unlock()
	var words []string
	if tool, ok := bridge.FindTool(tools, frame.Tool); ok {
		// Arguments that are no command's words are recorded as such: the
		// request carries the same and is refused for them.
		words, _ = tool.Words(frame.Arguments)
	}
	b.handler.Tool.CallSeen(harness.ObservedCall{ID: frame.Call, Conversation: thread, Turn: frame.Turn, Words: words, Nested: frame.Nested})
	return Frame{OK: true}
}

// toolResult reports the result the program handed the model for a call.
func (b *backend) toolResult(l *link, frame Frame) Frame {
	if !b.liveOn(l, ToolTransport) {
		return Frame{Error: "no live tool transport"}
	}
	if frame.Call == "" {
		return Frame{Error: "a tool result names its call"}
	}
	if !b.enter() {
		return Frame{Error: errClosing.Error()}
	}
	defer b.leave()
	if b.handler.Tool == nil {
		return Frame{Error: "the wrapper takes no tool results"}
	}
	b.handler.Tool.CallResult(frame.Call, harness.ToolResult{
		Succeeded: !frame.IsError, Direct: !frame.Nested, Texts: frame.Texts, Shortened: frame.Shortened,
	})
	return Frame{OK: true}
}
