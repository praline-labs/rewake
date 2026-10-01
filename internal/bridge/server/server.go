/*
Package server is the mail tool's server (docs/mail-bridge-server.md): a local
stdio MCP server the harness starts outside its sandbox. It is a transport. It
checks a call's words, gets the call's ticket from the run's wrapper, runs one
child of its own image with those words, and returns that child's answer. It
keeps nothing a later call needs: what any call did lives in the CLI's receipt
journal, so a server that dies, restarts or runs twice loses nothing.
*/
package server

import (
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/bridge/endpoint"
	"github.com/praline-labs/rewake/internal/state"
)

// Config is what the server is started with.
type Config struct {
	// Dir, Name and Epoch name the run, from the launch values.
	Dir, Name, Epoch string
	// Capability is the per-launch secret the endpoint checks.
	Capability string
	// Endpoint is the run's context socket.
	Endpoint string
	// Words checks a call's words on the tool's surface: their normalized
	// form, or the CLI's own refusal.
	Words func([]string) ([]string, string)
	// Executable is the image every child runs; Env their environment.
	Executable string
	Env        []string
	// Version is what the server says it is at initialize.
	Version string
	// TicketWait bounds a ticket request; zero is the endpoint's wait for
	// an observation and a second more.
	TicketWait time.Duration
}

// maxChildren is how many calls run at once; a fifth is refused as busy.
const maxChildren = 4

// Server runs until its stdin ends.
type Server struct {
	cfg   Config
	out   *encoder
	slots chan struct{}
	calls sync.WaitGroup

	// ending closes when stdin ends: from then on a child that outlives
	// its kill is waited for only reapWait more.
	ending chan struct{}

	mu       sync.Mutex
	ready    bool
	client   *endpoint.Client
	canceled map[string]bool
}

// Run serves the MCP protocol on stdin and stdout until stdin ends, then
// waits for its children up to their hard bounds, and for one that outlives
// its kill reapWait more; it leaves that one behind. It reads stdin apart
// from writing stdout, so a client that stops reading cannot hide the end of
// its stdin, and output stuck for outputWait ends it at once
// (docs/mail-bridge-server.md#running-the-child).
func Run(cfg Config, stdin io.Reader, stdout io.Writer) error {
	if cfg.TicketWait == 0 {
		cfg.TicketWait = 3 * time.Second
	}
	s := &Server{cfg: cfg, out: newEncoder(stdout), slots: make(chan struct{}, maxChildren), ending: make(chan struct{}), canceled: map[string]bool{}}
	s.connect()
	defer func() {
		s.mu.Lock()
		if s.client != nil {
			s.client.Close()
		}
		s.mu.Unlock()
	}()
	in := s.read(stdin)
	go func() {
		for frame := range in.frames {
			if frame == nil {
				s.out.fail(nil, codeInvalidRequest, "the request is longer than a request can be; nothing ran")
				continue
			}
			s.dispatch(frame)
		}
		close(in.dispatched)
	}()
	watch := time.NewTicker(watchEvery)
	defer watch.Stop()
	for {
		select {
		case <-in.dispatched:
			return s.finish(in.err)
		case <-watch.C:
			// Input that waits behind stuck output, or the end of stdin
			// the dispatch cannot reach: the transport is gone.
			if (in.backedUp.Load() || in.ended.Load()) && s.out.stuck(outputWait) {
				return s.finish(nil)
			}
		}
	}
}

// outputWait is how long output may stay stuck once the client has nothing
// more to say or cannot say it: past it the server ends. While the client
// still writes and its input does not back up, stuck output only holds the
// calls that answer into it.
const outputWait = 2 * time.Second

// watchEvery is how often Run looks at its output.
const watchEvery = 50 * time.Millisecond

// finish ends the server: no new call, its calls waited for up to their
// bounds, and its output while it moves. Output stuck for outputWait ends it
// with calls still open, as a server killed outright: what their children
// did is in their receipts, and an answer that never reached the client is
// no evidence of a read. A client that stopped reading has ended the
// conversation as much as one that closed stdin, so the exit is the same.
func (s *Server) finish(err error) error {
	s.mu.Lock()
	close(s.ending)
	s.mu.Unlock()
	done := make(chan struct{})
	go func() {
		s.calls.Wait()
		close(done)
	}()
	watch := time.NewTicker(watchEvery)
	defer watch.Stop()
	for {
		select {
		case <-done:
			return err
		case <-watch.C:
			if s.out.stuck(outputWait) {
				return err
			}
		}
	}
}

// connect says hello to the run's wrapper; a server whose hello failed tries
// again at its next call.
func (s *Server) connect() *endpoint.Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		select {
		case <-s.client.Gone():
			s.client.Close()
			s.client = nil
		default:
			return s.client
		}
	}
	client, err := endpoint.Dial(s.cfg.Endpoint, s.cfg.Capability)
	if err == nil {
		s.client = client
	}
	return s.client
}

// incoming is a JSON-RPC message the server reads.
type incoming struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func (s *Server) dispatch(frame []byte) {
	var m incoming
	if err := json.Unmarshal(frame, &m); err != nil {
		s.out.fail(nil, codeParse, "the request is not JSON; nothing ran")
		return
	}
	notification := len(m.ID) == 0
	if !notification && !validID(m.ID) {
		s.out.fail(nil, codeInvalidRequest, "the request id is longer than an id can be; nothing ran")
		return
	}
	switch m.Method {
	case "initialize":
		s.initialize(m)
	case "notifications/initialized":
	case "notifications/cancelled": //nolint:misspell // the protocol names the method so
		s.cancel(m.Params)
	case "ping":
		s.out.send(message{Version: "2.0", ID: m.ID, Result: struct{}{}})
	case "tools/list":
		s.out.send(message{Version: "2.0", ID: m.ID, Result: toolList})
	case "tools/call":
		s.call(m)
	default:
		if !notification {
			s.out.fail(m.ID, codeNoMethod, "the rewake server answers only initialize, ping, tools/list and tools/call")
		}
	}
}

func (s *Server) initialize(m incoming) {
	var asked struct {
		Version string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(m.Params, &asked)
	if asked.Version == "" || len(asked.Version) > 32 {
		asked.Version = "2025-06-18"
	}
	s.mu.Lock()
	s.ready = true
	s.mu.Unlock()
	s.out.send(message{Version: "2.0", ID: m.ID, Result: map[string]any{
		"protocolVersion": asked.Version,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": "rewake", "version": s.cfg.Version},
	}})
}

// toolList is the one tool.
var toolList = map[string]any{"tools": []any{map[string]any{
	"name":        "rewake",
	"description": "Run a rewake mail command: its words, as a shell would pass them after the program name, such as [\"inbox\"].",
	"inputSchema": map[string]any{
		"type":                 "object",
		"properties":           map[string]any{"words": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}},
		"required":             []string{"words"},
		"additionalProperties": false,
	},
}}}

// cancel drops the answer of a call the harness gave up on; its child runs
// on, and its receipt holds the outcome.
func (s *Server) cancel(params json.RawMessage) {
	var asked struct {
		ID json.RawMessage `json:"requestId"`
	}
	if json.Unmarshal(params, &asked) != nil || !validID(asked.ID) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.canceled) < 64 {
		s.canceled[string(asked.ID)] = true
	}
}

// callParams is a tools/call's params as the server reads them.
type callParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Meta      struct {
		CodexCall   string `json:"callId"`
		CodexThread string `json:"threadId"`
		CodexTurn   struct {
			Turn string `json:"turn_id"`
		} `json:"x-codex-turn-metadata"`
		ClaudeCall string `json:"claudecode/toolUseId"`
	} `json:"_meta"`
}

func (s *Server) call(m incoming) {
	s.mu.Lock()
	ready := s.ready
	s.mu.Unlock()
	if !ready {
		s.out.fail(m.ID, codeInvalidRequest, "tools/call before initialize; nothing ran")
		return
	}
	var params callParams
	if json.Unmarshal(m.Params, &params) != nil || params.Name != "rewake" {
		s.out.fail(m.ID, codeInvalidParams, "the rewake server has one tool, rewake; nothing ran")
		return
	}
	words, ok := wordsOf(params.Arguments)
	if !ok {
		s.out.fail(m.ID, codeInvalidParams, "the rewake tool takes one argument, words, an array of strings; nothing ran")
		return
	}
	arrived := boottime.Now()
	select {
	case s.slots <- struct{}{}:
	default:
		s.out.reply(m.ID, substitute(busy))
		return
	}
	// A frame dispatched after the server began to end runs nothing, so
	// that every call finish waits for began before it.
	s.mu.Lock()
	select {
	case <-s.ending:
		s.mu.Unlock()
		<-s.slots
		s.out.reply(m.ID, substitute(stopping))
		return
	default:
	}
	s.calls.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.calls.Done()
		defer func() { <-s.slots }()
		a := s.run(params, words, arrived)
		s.mu.Lock()
		dropped := s.canceled[string(m.ID)]
		delete(s.canceled, string(m.ID))
		s.mu.Unlock()
		if dropped {
			return
		}
		state.Step("answer")
		s.out.reply(m.ID, a)
	}()
}

const stopping = "Rewake: the tool's server is ending, so nothing ran; run the same words in the shell.\n"

const busy = "Rewake: four tool calls are running already, so nothing ran; call again in a moment, or run the same words in the shell.\n"

// run takes one call from its words to its answer.
func (s *Server) run(params callParams, words []string, arrived int64) answer {
	normalized, refusal := s.cfg.Words(words)
	if refusal != "" {
		return answer{stderr: refusal, code: 2}
	}
	asked := endpoint.TicketRequest{Words: normalized, Digest: bridge.Digest(normalized), Arrived: arrived}
	meta := params.Meta
	switch {
	case meta.ClaudeCall != "" && meta.CodexCall == "":
		asked.Transport, asked.CallID = bridge.ClaudeTransport, meta.ClaudeCall
	case meta.CodexCall != "" && meta.ClaudeCall == "":
		asked.Transport, asked.CallID = bridge.CodexTransport, meta.CodexCall
		asked.Conversation, asked.Turn = meta.CodexThread, meta.CodexTurn.Turn
	default:
		return substitute("Rewake: this call carries no native ids the wrapper can match, so nothing ran; run the same words in the shell.\n")
	}
	client := s.connect()
	if client == nil {
		return substitute("Rewake: this run's wrapper does not answer the tool, so nothing ran; run the same words in the shell.\n")
	}
	state.Step("ticket")
	ticket, err := client.Ticket(asked, s.cfg.TicketWait)
	if err != nil {
		return substitute("Rewake: the wrapper issued no ticket for this call (" + err.Error() + "), so nothing ran; run the same words in the shell.\n")
	}
	return s.runChild(ticket, normalized)
}

// wordsOf reads a call's arguments: one key, words, an array of strings.
func wordsOf(arguments json.RawMessage) ([]string, bool) {
	var shape map[string]json.RawMessage
	if json.Unmarshal(arguments, &shape) != nil || len(shape) != 1 {
		return nil, false
	}
	var words []string
	if json.Unmarshal(shape["words"], &words) != nil || words == nil {
		return nil, false
	}
	return words, true
}

// ChildEnv is the environment of every child, from the server's own.
func ChildEnv(getenv func(string) string) []string { return childEnv(getenv) }
