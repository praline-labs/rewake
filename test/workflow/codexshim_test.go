package workflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
)

// The shim plays a Codex *session*, not a transport.
//
// The wrapper starts two halves: an app-server listening on <socket>.up, and a
// client connected to <socket>, which is the wrapper's own proxy. The gateway
// only accepts a conversation after it recognizes the client's request and
// sees the server's reply, so a shim that answers only one half proves
// nothing: delivery would wait for a conversation that was never accepted.
//
// It is meant to be an honest peer. Where it answers differently from the real
// server, that is a deliberate control named in the environment — and if the
// gateway then still reports readiness, that is a finding about rewake, not a
// reason to make the shim more agreeable.

const (
	shimEnv = "RW_SHIM"
	// shimThread is the conversation the shim starts. A numeric request id and
	// threadSource=user are what make the request recognizable to the
	// gateway; the thread id itself only has to be stable.
	shimThread = "0199ab12-0000-7000-8000-abcdef000001"

	// Controls. Each one makes the shim answer like a server that is wrong in
	// one specific way, so a scenario can check that readiness does not follow.
	shimNoDirectInput = "RW_SHIM_NO_DIRECT_INPUT"
	shimWrongThread   = "RW_SHIM_WRONG_THREAD"
	// shimNoCorrelatedReply announces a conversation as an event and never
	// answers the request that asked for it.
	shimNoCorrelatedReply = "RW_SHIM_NO_CORRELATED_REPLY"
	// shimResume makes the client continue a named conversation instead of
	// starting a new one. Only a continuation carries a conversation id the
	// client chose, so that is where "the server answered about a different
	// conversation" can be asked at all.
	shimResume = "RW_SHIM_RESUME"

	// shimStateFile is where the client half writes what `rewake list` says.
	// The session runs the command, not the test: that is the only way the
	// provenance of an observation can be the session's.
	shimStateFile = "RW_SHIM_STATE_FILE"
	// shimAcceptedFile is where the client writes the conversation id the
	// server actually gave it.
	shimAcceptedFile = "RW_SHIM_ACCEPTED_FILE"
)

// runShim is the entry point when this binary is re-executed as `codex`.
func runShim(args []string) int {
	for _, arg := range args {
		if arg == "--version" {
			fmt.Println(lastObservedServerVersionForShim())
			return 0
		}
	}
	if socket, ok := flagValue(args, "--listen"); ok {
		return shimServer(strings.TrimPrefix(socket, "unix://"))
	}
	if socket, ok := flagValue(args, "--remote"); ok {
		return shimClient(strings.TrimPrefix(socket, "unix://"))
	}
	fmt.Fprintln(os.Stderr, "shim: neither --listen nor --remote")
	return 2
}

// lastObservedServerVersionForShim answers the wrapper's version probe. It is
// a literal rather than the adapter's constant: the shim is standing in for a
// program that reports its own version, and echoing ours back would make the
// wrapper's check agree with itself.
func lastObservedServerVersionForShim() string { return "codex-cli 0.155.1" }

func flagValue(args []string, name string) (string, bool) {
	for i, arg := range args {
		if arg == name && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

// shimServer is the app-server half.
func shimServer(socket string) int {
	listener, err := net.Listen("unix", socket)
	if err != nil {
		fmt.Fprintf(os.Stderr, "shim: listen %s: %v\n", socket, err)
		return 3
	}
	defer func() { _ = listener.Close() }()

	session := &shimSession{thread: shimThread}
	for {
		conn, err := listener.Accept()
		if err != nil {
			return 0
		}
		go session.serve(conn)
	}
}

// shimSession is what the served conversation looks like. Negotiation is not
// here: it belongs to each connection, because a second client that never
// initialized must not inherit the first one's handshake.
type shimSession struct {
	mu     sync.Mutex
	thread string
	peers  []*shimPeer
}

// shimPeer is one connection and what it negotiated.
type shimPeer struct {
	ws           *wsConn
	initialized  bool
	experimental bool
}

func (s *shimSession) serve(conn net.Conn) {
	ws, err := wsAccept(conn)
	if err != nil {
		_ = conn.Close()
		return
	}
	defer ws.close()
	peer := &shimPeer{ws: ws}
	s.mu.Lock()
	s.peers = append(s.peers, peer)
	s.mu.Unlock()

	for {
		raw, err := ws.readMessage()
		if err != nil {
			return
		}
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(raw, &request) != nil {
			return
		}
		if request.Method == "initialized" {
			continue
		}
		result, after, failure := s.answer(peer, request.Method, request.Params)
		if failure == nil && result == nil {
			// Deliberate silence, not an empty result: a reply of any shape,
			// including {"result":null}, is still a correlated reply, and the
			// control that means "no answer came" has to send nothing at all.
			continue
		}
		if len(request.ID) == 0 {
			continue
		}
		reply := map[string]any{"id": json.RawMessage(request.ID)}
		if failure != nil {
			reply["error"] = map[string]any{"code": -32600, "message": failure.Error()}
		} else {
			reply["result"] = result
		}
		if err := ws.writeJSON(reply); err != nil {
			return
		}
		// Events that follow a reply are sent after it, which is the order the
		// real server uses: a client learns its conversation from its own
		// reply, not from an event that might belong to somebody else.
		if after != nil {
			s.mu.Lock()
			s.broadcast(after)
			s.mu.Unlock()
		}
	}
}

// answer returns the reply, an event to send after it, or a refusal. A nil
// reply with no error means "send nothing", which only the missing-reply
// control uses.
func (s *shimSession) answer(peer *shimPeer, method string, params json.RawMessage) (any, any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if method == "initialize" {
		result, err := peer.initialize(params)
		return result, nil, err
	}
	if !peer.initialized {
		// The real server negotiates per connection before it serves.
		return nil, nil, errors.New("initialize has not been received on this connection")
	}
	if !isObject(params) {
		// Every method here takes an object. A request whose params are an
		// array or a scalar is malformed, and a server that answers it
		// successfully is inventing a contract.
		return nil, nil, errors.New("params must be an object")
	}
	if !peer.experimental && usesExperimental(params) {
		// runtimeWorkspaceRoots and canAcceptDirectInput are experimental. A
		// server that serves them to a client which never declared the
		// capability is modeling nothing real — and this is checked for every
		// method, not only the one where it was first noticed.
		return nil, nil, errors.New("experimental fields were used without negotiating them")
	}
	if bad := malformedField(params); bad != "" {
		return nil, nil, errors.New(bad)
	}
	switch method {
	case "thread/start":
		// ThreadStartParams has no conversation id in 0.155.1: the server
		// names a new conversation and the client learns it from the reply.
		if hasField(params, "threadId") {
			return nil, nil, errors.New("thread/start does not take a threadId")
		}
		return s.lifecycle(s.thread)
	case "thread/resume":
		asked := threadOf(params)
		if asked == "" {
			return nil, nil, errors.New("thread/resume needs a threadId")
		}
		if asked != s.thread {
			// Resuming a conversation this server does not have is a refusal,
			// not a success about some other conversation.
			return nil, nil, fmt.Errorf("no such thread %q", asked)
		}
		return s.lifecycle(asked)
	case "thread/read":
		asked := threadOf(params)
		if asked == "" {
			return nil, nil, errors.New("thread/read needs a threadId")
		}
		if asked != s.thread {
			return nil, nil, fmt.Errorf("no such thread %q", asked)
		}
		return map[string]any{"thread": s.threadDescription(asked)}, nil, nil
	case "thread/unsubscribe", "thread/settings/update", "thread/metadata/update":
		// Not implemented, so refused. An unconditional success for a method
		// whose contract this shim does not model is the same invention as
		// answering an unknown method — and the gateway does call these.
		return nil, nil, fmt.Errorf("the shim does not implement %s", method)
	default:
		// Refusing is honest; inventing a success for a contract this shim
		// does not implement is how a fixture starts proving untrue things.
		return nil, nil, fmt.Errorf("the shim does not implement %s", method)
	}
}

// lifecycle answers a start or resume, and names the event that follows it.
func (s *shimSession) lifecycle(id string) (any, any, error) {
	thread := s.threadDescription(id)
	if os.Getenv(shimNoCorrelatedReply) != "" {
		// Announce a conversation and never answer the request that asked for
		// it. "A conversation started" is not "your request was accepted".
		s.broadcast(map[string]any{"method": "thread/started", "params": map[string]any{"thread": thread}})
		return nil, nil, nil
	}
	return s.startResponse(thread),
		map[string]any{"method": "thread/started", "params": map[string]any{"thread": thread}},
		nil
}

// startResponse is ThreadStartResponse as the schema of 0.155.1 defines it:
// approvalPolicy, approvalsReviewer, cwd, model, modelProvider, sandbox and
// the thread are all required.
func (s *shimSession) startResponse(thread map[string]any) map[string]any {
	return map[string]any{
		"thread":            thread,
		"approvalPolicy":    "on-request",
		"approvalsReviewer": "user",
		"cwd":               "/work",
		"model":             "shim-model",
		"modelProvider":     "openai",
		// SandboxPolicy is a tagged object, not the bare mode string: the
		// mode name belongs to SandboxMode, which this field is not.
		"sandbox": map[string]any{"type": "workspaceWrite"},
	}
}

// initialize negotiates for this connection, and answers InitializeResponse:
// codexHome, platformFamily, platformOs and userAgent are required.
func (p *shimPeer) initialize(params json.RawMessage) (any, error) {
	var asked struct {
		ClientInfo struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"clientInfo"`
		Capabilities struct {
			ExperimentalAPI bool `json:"experimentalApi"`
		} `json:"capabilities"`
	}
	if p.initialized {
		// The real server refuses a second handshake rather than quietly
		// replacing what the connection negotiated.
		return nil, errors.New("this connection is already initialized")
	}
	if json.Unmarshal(params, &asked) != nil {
		return nil, errors.New("unreadable initialize params")
	}
	if asked.ClientInfo.Name == "" || asked.ClientInfo.Version == "" {
		return nil, errors.New("clientInfo needs both name and version")
	}
	p.initialized = true
	p.experimental = asked.Capabilities.ExperimentalAPI
	return map[string]any{
		"codexHome":      os.Getenv("CODEX_HOME"),
		"platformFamily": "unix",
		"platformOs":     "linux",
		"userAgent":      "rewake-shim/0.0.1",
	}, nil
}

// threadDescription is the Thread object. Its required fields come from the
// schema of 0.155.1; canAcceptDirectInput belongs inside it rather than beside
// it, which is the branch of the adapter's parser that matters.
func (s *shimSession) threadDescription(id string) map[string]any {
	if os.Getenv(shimWrongThread) != "" {
		// A server that answers about a different conversation than the one
		// that was asked for. The gateway must not treat that as acceptance.
		id = "0199ab12-ffff-7fff-8fff-ffffffffffff"
	}
	thread := map[string]any{
		"id":            id,
		"sessionId":     id,
		"cliVersion":    "0.155.1",
		"createdAt":     1758000000,
		"updatedAt":     1758000000,
		"cwd":           "/work",
		"ephemeral":     false,
		"modelProvider": "openai",
		"preview":       "",
		"projectId":     nil,
		"source":        "cli",
		"threadSource":  "user",
		"originator":    "rewake",
		"status":        map[string]string{"type": "idle"},
		"turns":         []any{},
	}
	if os.Getenv(shimNoDirectInput) == "" {
		thread["canAcceptDirectInput"] = true
	}
	return thread
}

// broadcast sends an event to every connected peer. Call with the lock held.
func (s *shimSession) broadcast(event any) {
	for _, peer := range s.peers {
		_ = peer.ws.writeJSON(event)
	}
}

// threadOf reads the conversation id a request names, if any.
func threadOf(params json.RawMessage) string {
	var asked struct {
		ThreadID string `json:"threadId"`
	}
	if json.Unmarshal(params, &asked) != nil {
		return ""
	}
	return asked.ThreadID
}

// usesExperimental reports whether a request carries a field that only an
// experimental client may use.
func usesExperimental(params json.RawMessage) bool {
	return hasField(params, "runtimeWorkspaceRoots")
}

// The shape of a request is checked explicitly, not by whether Go's decoder
// complained. It accepts JSON null into almost anything — a null object
// becomes an empty map, a null array element becomes an empty string — so
// "it parsed" says very little about what arrived.

// jsonKind reports what a raw JSON value is, or "" when it is unreadable.
func jsonKind(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || !json.Valid(trimmed) {
		return ""
	}
	switch trimmed[0] {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	default:
		return "number"
	}
}

// isObject reports whether params is a JSON object — null is not one.
func isObject(params json.RawMessage) bool { return jsonKind(params) == "object" }

// malformedField names the first field whose shape is wrong for the methods
// this shim serves.
func malformedField(params json.RawMessage) string {
	var fields map[string]json.RawMessage
	if json.Unmarshal(params, &fields) != nil {
		return "params"
	}
	if raw, ok := fields["config"]; ok && jsonKind(raw) != "object" && jsonKind(raw) != "null" {
		return "config must be an object"
	}
	if raw, ok := fields["threadSource"]; ok && jsonKind(raw) != "string" {
		return "threadSource must be a string"
	}
	if raw, ok := fields["threadId"]; ok && jsonKind(raw) != "string" {
		return "threadId must be a string"
	}
	if raw, ok := fields["runtimeWorkspaceRoots"]; ok {
		if jsonKind(raw) != "array" {
			return "runtimeWorkspaceRoots must be a list of paths"
		}
		var roots []json.RawMessage
		if json.Unmarshal(raw, &roots) != nil {
			return "runtimeWorkspaceRoots must be a list of paths"
		}
		for _, root := range roots {
			if jsonKind(root) != "string" {
				return "every runtimeWorkspaceRoots entry must be a path"
			}
		}
	}
	return ""
}

// hasField reports whether a JSON object carries a key at all.
func hasField(params json.RawMessage, name string) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(params, &fields) != nil {
		return false
	}
	_, ok := fields[name]
	return ok
}
