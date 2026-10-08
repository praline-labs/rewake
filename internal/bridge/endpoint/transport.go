package endpoint

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/state"
)

// A transport's tool call (docs/v2/design-api.md#tooltransport):
// the harness's own process asks the endpoint to run one call, with the
// binding it made from its own ids, and gets the answer back to hand its
// model. The endpoint makes the ticket, runs the call in a child of the
// wrapper's own image, and bounds the answer; the transport runs nothing.
// Each request proves its origin on its own: one request per connection,
// whose peer must be the transport's process itself — by pid and start time,
// before the call and again before its answer — never a descendant, since a
// command the model runs is one. No secret carries authority.

// ToolCall is one call a transport asks to run: the tool and its arguments
// as the model gave them, and the binding from the harness's own ids.
type ToolCall struct {
	Tool         string          `json:"tool"`
	Arguments    json.RawMessage `json:"arguments,omitempty"`
	CallID       string          `json:"callId"`
	Conversation string          `json:"conversation"`
	Turn         string          `json:"turn"`
	// TurnsNeverReused is the transport's declaration that it never gives
	// a turn id to another turn (TicketRequest).
	TurnsNeverReused bool `json:"turnsNeverReused,omitempty"`
}

// maxCall bounds a call's request before it is parsed: the largest arguments
// a descriptor takes, and the binding beside them.
const maxCall = bridge.MaxWordsBytes + 16<<10

// transportPeer is the process a transport's requests must come from.
type transportPeer struct {
	pid   int
	start uint64
}

// SetTransport names the transport's process, by pid and start time: the
// only process whose requests run calls. Until it is called none does.
func (e *Endpoint) SetTransport(pid int, start uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.transport = transportPeer{pid: pid, start: start}
}

// SetChildEnv names the environment of a transport's call: the run's launch
// values from the harness's environment, as a server's children get them.
func (e *Endpoint) SetChildEnv(getenv func(string) string) {
	env := ChildEnv(getenv)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.childEnv = env
}

// fromTransport says whether pid is the transport's process, alive with the
// start it was named with.
func (e *Endpoint) fromTransport(pid int) error {
	e.mu.Lock()
	peer := e.transport
	e.mu.Unlock()
	if peer.pid == 0 {
		return errors.New("this run has no tool transport")
	}
	if pid != peer.pid {
		return fmt.Errorf("process %d is not the harness's own process; nothing it asks runs", pid)
	}
	if start, err := proc.StartTime(pid); err != nil || start != peer.start {
		return errors.New("the harness's process is gone or was replaced")
	}
	return nil
}

// serveCall answers a transport's one request: bounded before it is parsed,
// run, and answered only while its peer is still the transport's process.
func (e *Endpoint) serveCall(conn *net.UnixConn, reader *bufio.Reader, writer *lineWriter, pid int) {
	line, err := readBounded(reader, maxCall)
	if errors.Is(err, errOverBound) {
		writer.write(response{Error: "the request is longer than a request can be; nothing ran"})
		return
	}
	if err != nil {
		return
	}
	var asked request
	if json.Unmarshal(line, &asked) != nil || asked.Op != opCall || asked.Call == nil {
		writer.write(response{ID: asked.ID, Error: "a transport asks only to run a call; nothing ran"})
		return
	}
	// The hello proved the peer when it came; the request is admitted only
	// if the peer is still the transport's process as it arrives.
	if err := e.fromTransport(pid); err != nil {
		writer.write(response{ID: asked.ID, Error: err.Error() + "; nothing ran"})
		return
	}
	// The call is bounded by its child's deadline, not by the exchange's.
	_ = conn.SetDeadline(time.Time{})
	answer := e.runCall(*asked.Call, boottime.Now())
	if err := e.fromTransport(pid); err != nil {
		// The process that asked is gone: nobody is there to hand the
		// answer to a model. What the call did is in its receipt.
		return
	}
	e.step("answer")
	_ = conn.SetWriteDeadline(time.Now().Add(shortExchange))
	writer.write(response{ID: asked.ID, Answer: &answer})
}

// runCall takes one call from its arguments to its answer.
func (e *Endpoint) runCall(call ToolCall, arrived int64) ToolAnswer {
	tool, ok := bridge.FindTool(e.cfg.Tools, call.Tool)
	if !ok {
		return substitute("Rewake: this run offers no tool named " + quoted(call.Tool) + ", so nothing ran.\n").encoded()
	}
	words, err := tool.Words(call.Arguments)
	if err != nil {
		return childAnswer{stderr: "Rewake: " + err.Error() + "; nothing ran.\n", code: 2}.encoded()
	}
	if e.cfg.Check == nil {
		return substitute("Rewake: this run cannot check a call's words, so nothing ran; run the same words in the shell.\n").encoded()
	}
	normalized, refusal := e.cfg.Check(words)
	if refusal != "" {
		return childAnswer{stderr: refusal, code: 2}.encoded()
	}
	select {
	case e.slots <- struct{}{}:
	default:
		return substitute(busy).encoded()
	}
	defer func() { <-e.slots }()
	e.step("ticket")
	ticket, err := e.issue(TicketRequest{
		Transport: e.cfg.Transport, Conversation: call.Conversation, Turn: call.Turn, CallID: call.CallID,
		TurnsNeverReused: call.TurnsNeverReused, Words: normalized, Digest: bridge.Digest(normalized), Arrived: arrived,
	}, 0)
	if err != nil {
		return substitute("Rewake: the wrapper issued no ticket for this call (" + err.Error() + "), so nothing ran; run the same words in the shell.\n").encoded()
	}
	return e.runChild(ticket, normalized).encoded()
}

// quoted is a name as an answer shows it, cut to a line.
func quoted(name string) string {
	if len(name) > 64 {
		name = name[:64]
	}
	encoded, _ := json.Marshal(name)
	return string(encoded)
}

const busy = "Rewake: four tool calls are running already, so nothing ran; call again in a moment, or run the same words in the shell.\n"

// errOverBound is a line longer than its bound.
var errOverBound = errors.New("a line over its bound")

// readBounded reads one line within limit bytes, and stops reading past it.
func readBounded(reader *bufio.Reader, limit int) ([]byte, error) {
	var line []byte
	for {
		chunk, more, err := reader.ReadLine()
		if err != nil {
			return nil, err
		}
		line = append(line, chunk...)
		if len(line) > limit {
			return nil, errOverBound
		}
		if !more {
			return line, nil
		}
	}
}

// step names a step of this run's call path to the fault seam, by the run's
// context path: a test that plays several wrappers in one process tells their
// steps apart by it, as it tells their files apart by their paths.
func (e *Endpoint) step(name string) { state.Step(e.path + "/" + name) }
