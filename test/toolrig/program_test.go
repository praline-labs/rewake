//go:build rewakefixture

package toolrig

// The program the fixture adapter runs in the rig: the harness's side of every
// call, as the Claude Code mod will play it. It is this test binary again,
// started by the adapter's backend through a launcher the rig writes; it says
// hello on the adapter's socket, answers the probes, and then does what the rig
// asks over a control socket of its own, one step of a call at a time — the
// native call observed, the request, the result recorded — so the rig can
// hold, drop, repeat or reorder each step, or end the process at one.
//
// The adapter's exchange is spelled out here rather than imported, as the
// workflow suite's program does: a program sharing the adapter's types would
// agree with it by construction.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
)

// The launcher's variables: the control socket the program serves the rig on,
// its fault plan, and its switches.
const (
	controlEnv = "TOOLRIG_CONTROL"
	faultEnv   = "TOOLRIG_FAULT"
	// unprovenEnv makes the transport one that cannot show the result it
	// handed the model.
	unprovenEnv = "TOOLRIG_UNPROVEN"
	// reusedEnv withholds the declaration that the program's turn ids are
	// never reused.
	reusedEnv = "TOOLRIG_TURNS_REUSED"
)

// thread is the conversation the program reports.
const thread = "conversation"

// frame is one line of the adapter's exchange.
type frame struct {
	Op         string                  `json:"op"`
	ID         int64                   `json:"id,omitempty"`
	Version    string                  `json:"version,omitempty"`
	PID        int                     `json:"pid,omitempty"`
	Thread     string                  `json:"thread,omitempty"`
	Serves     []string                `json:"serves,omitempty"`
	Capability string                  `json:"capability,omitempty"`
	OK         bool                    `json:"ok,omitempty"`
	Error      string                  `json:"error,omitempty"`
	State      string                  `json:"state,omitempty"`
	Turn       string                  `json:"turn,omitempty"`
	End        string                  `json:"end,omitempty"`
	Outcome    string                  `json:"outcome,omitempty"`
	At         int64                   `json:"at,omitempty"`
	Tools      []bridge.ToolDescriptor `json:"tools,omitempty"`
	Endpoint   string                  `json:"endpoint,omitempty"`
	Names      []string                `json:"names,omitempty"`
	Digest     string                  `json:"digest,omitempty"`
	Call       string                  `json:"call,omitempty"`
	Tool       string                  `json:"tool,omitempty"`
	Arguments  json.RawMessage         `json:"arguments,omitempty"`
	Nested     bool                    `json:"nested,omitempty"`
	Texts      []string                `json:"texts,omitempty"`
	IsError    bool                    `json:"isError,omitempty"`
	Shortened  bool                    `json:"shortened,omitempty"`
}

// command is one request of the rig over the control socket.
type command struct {
	Op        string          `json:"op"`
	Turn      string          `json:"turn,omitempty"`
	End       string          `json:"end,omitempty"`
	At        int64           `json:"at,omitempty"`
	Call      string          `json:"call,omitempty"`
	Tool      string          `json:"tool,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Nested    bool            `json:"nested,omitempty"`
	Texts     []string        `json:"texts,omitempty"`
	IsError   bool            `json:"isError,omitempty"`
	Shortened bool            `json:"shortened,omitempty"`
	// Raw is a request line sent as it is, in place of a call's.
	Raw string `json:"raw,omitempty"`
	// ReadAfter delays reading the answer; Unread closes the connection
	// without reading it.
	ReadAfter time.Duration `json:"readAfter,omitempty"`
	Unread    bool          `json:"unread,omitempty"`
}

// reply answers a command: the call's answer, the endpoint's refusal, or
// the adapter's.
type reply struct {
	Texts   []string `json:"texts,omitempty"`
	IsError bool     `json:"isError,omitempty"`
	Error   string   `json:"error,omitempty"`
	// Raw is the endpoint's answer line to a raw request.
	Raw string `json:"raw,omitempty"`
}

// program is the running program: its link to the adapter and what the
// probe offered it.
type program struct {
	conn     net.Conn
	writeMu  sync.Mutex
	mu       sync.Mutex
	next     int64
	waiting  map[int64]chan frame
	endpoint string
	// offered are the descriptors the probe offered.
	offered []bridge.ToolDescriptor
	faults  *transportFaults
}

func runProgram(args []string) int {
	connect := ""
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--connect" {
			connect = args[i+1]
		}
	}
	if connect == "" {
		fmt.Fprintln(os.Stderr, "toolrig program: no --connect")
		return 2
	}
	p := &program{waiting: map[int64]chan frame{}, faults: parseFaults(os.Getenv(faultEnv))}
	control, err := net.Listen("unix", os.Getenv(controlEnv))
	if err != nil {
		fmt.Fprintln(os.Stderr, "toolrig program:", err)
		return 1
	}
	conn, err := net.Dial("unix", connect)
	if err != nil {
		fmt.Fprintln(os.Stderr, "toolrig program:", err)
		return 1
	}
	p.conn = conn
	p.write(frame{Op: "hello", Version: "1.0.0", PID: os.Getpid(), Thread: thread, Serves: []string{"turn-boundary", "tool-transport"}})
	go p.serveControl(control)
	reader := bufio.NewReaderSize(conn, 1<<20)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			// The adapter closed: the program's run is over.
			return 0
		}
		var f frame
		if json.Unmarshal(line, &f) != nil {
			continue
		}
		p.handle(f)
	}
}

func (p *program) write(f frame) {
	encoded, _ := json.Marshal(f)
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	_, _ = p.conn.Write(append(encoded, '\n'))
}

// handle takes one frame of the adapter: an answer to the program's own
// request, or a probe.
func (p *program) handle(f frame) {
	if f.Op == "answer" {
		p.mu.Lock()
		waiter := p.waiting[f.ID]
		delete(p.waiting, f.ID)
		p.mu.Unlock()
		if waiter != nil {
			waiter <- f
		}
		return
	}
	if f.Op != "probe" {
		if f.ID != 0 {
			p.write(frame{Op: "answer", ID: f.ID, Error: "the rig's program does not take " + f.Op})
		}
		return
	}
	switch f.Capability {
	case "turn-boundary":
		p.write(frame{Op: "answer", ID: f.ID, OK: true, State: "idle"})
	case "tool-transport":
		names := make([]string, 0, len(f.Tools))
		for _, tool := range f.Tools {
			names = append(names, tool.Name)
		}
		p.mu.Lock()
		p.endpoint, p.offered = f.Endpoint, f.Tools
		p.mu.Unlock()
		state := "proven"
		if os.Getenv(unprovenEnv) != "" {
			state = "unproven"
		}
		p.write(frame{Op: "answer", ID: f.ID, OK: true, State: state, Names: names, Digest: bridge.DescriptorsDigest(f.Tools)})
	default:
		p.write(frame{Op: "answer", ID: f.ID, Error: "not served"})
	}
}

// ask sends a request to the adapter and waits for its answer.
func (p *program) ask(f frame) frame {
	p.mu.Lock()
	p.next++
	f.ID = p.next
	waiter := make(chan frame, 1)
	p.waiting[f.ID] = waiter
	p.mu.Unlock()
	p.write(f)
	select {
	case answer := <-waiter:
		return answer
	case <-time.After(30 * time.Second):
		return frame{Error: "the adapter did not answer"}
	}
}

func (p *program) serveControl(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go func() {
			defer func() { _ = conn.Close() }()
			line, err := bufio.NewReader(conn).ReadBytes('\n')
			if err != nil {
				return
			}
			var c command
			if json.Unmarshal(line, &c) != nil {
				return
			}
			encoded, _ := json.Marshal(p.run(c))
			_, _ = conn.Write(append(encoded, '\n'))
		}()
	}
}

// run does one command of the rig.
func (p *program) run(c command) reply {
	switch c.Op {
	case "turn-started":
		return adapterReply(p.ask(frame{Op: "turn-started", Turn: c.Turn, At: c.At}))
	case "turn-ended":
		return adapterReply(p.ask(frame{Op: "turn-ended", Turn: c.Turn, End: c.End, Outcome: "completed"}))
	case "observe":
		p.faults.step("observe")
		return adapterReply(p.ask(frame{Op: "tool-call", Call: c.Call, Turn: c.Turn, Tool: c.Tool, Arguments: c.Arguments, Nested: c.Nested}))
	case "result":
		p.faults.step("result")
		return adapterReply(p.ask(frame{Op: "tool-result", Call: c.Call, Texts: c.Texts, IsError: c.IsError, Shortened: c.Shortened, Nested: c.Nested}))
	case "offered":
		// What the probe offered: the names, and the digest of the whole.
		p.mu.Lock()
		defer p.mu.Unlock()
		names := make([]string, 0, len(p.offered))
		for _, tool := range p.offered {
			names = append(names, tool.Name)
		}
		return reply{Texts: names, Raw: bridge.DescriptorsDigest(p.offered)}
	case "request":
		p.faults.step("request")
		answer := p.request(c)
		p.faults.step("answered")
		return answer
	}
	return reply{Error: "no such command: " + c.Op}
}

func adapterReply(answer frame) reply {
	if !answer.OK {
		return reply{Error: "the adapter: " + answer.Error}
	}
	return reply{}
}

// request asks the endpoint to run one call, on a connection of its own, as
// the endpoint's transport role takes it.
func (p *program) request(c command) reply {
	p.mu.Lock()
	path := p.endpoint
	p.mu.Unlock()
	conn, err := net.Dial("unix", path)
	if err != nil {
		return reply{Error: err.Error()}
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(90 * time.Second))
	reader := bufio.NewReaderSize(conn, 1<<20)
	if _, err := conn.Write([]byte(`{"role":"transport"}` + "\n")); err != nil {
		return reply{Error: err.Error()}
	}
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return reply{Error: "the hello: " + err.Error()}
	}
	var greeted struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(line, &greeted) != nil || greeted.Error != "" {
		return reply{Error: "the hello: " + greeted.Error}
	}
	request := c.Raw
	if request == "" {
		call := map[string]any{"tool": c.Tool, "callId": c.Call, "conversation": thread, "turn": c.Turn, "turnsNeverReused": os.Getenv(reusedEnv) == ""}
		if c.Arguments != nil {
			call["arguments"] = c.Arguments
		}
		encoded, _ := json.Marshal(map[string]any{"id": 1, "op": "call", "call": call})
		request = string(encoded)
	}
	if _, err := conn.Write([]byte(request + "\n")); err != nil {
		return reply{Error: err.Error()}
	}
	p.faults.step("sent")
	if c.Unread {
		return reply{}
	}
	time.Sleep(c.ReadAfter)
	line, err = reader.ReadBytes('\n')
	if err != nil {
		return reply{Error: "no answer: " + err.Error()}
	}
	if c.Raw != "" {
		return reply{Raw: strings.TrimSuffix(string(line), "\n")}
	}
	var answered struct {
		Error  string `json:"error"`
		Answer *struct {
			Texts   []string `json:"texts"`
			IsError bool     `json:"isError"`
		} `json:"answer"`
	}
	if err := json.Unmarshal(line, &answered); err != nil {
		return reply{Error: "an answer that does not parse"}
	}
	if answered.Answer == nil {
		return reply{Error: "the endpoint: " + answered.Error}
	}
	return reply{Texts: answered.Answer.Texts, IsError: answered.Answer.IsError}
}
