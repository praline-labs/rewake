package workflow

// The program the fixture adapter runs: the third column's harness. It is the
// test binary again, as the other two are, and keeps the suite's
// test-to-harness protocol — the environment at launch and the files after
// it — so the neutral scenarios run on it unchanged.
//
// It has two halves, as Codex does. The adapter's backend starts the session
// half with --connect: it says hello on the adapter's socket, answers the
// probes of what it serves, takes notices and works its turns. The wrapper
// starts the terminal half: the session's foreground, which sends as asked,
// serves the scenario's requests and stays up until it is told to stop.
//
// The exchange is spelled out here rather than imported from the adapter: the
// adapter exists only in a tagged build, and a program sharing its types would
// agree with it by construction.

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
)

// The program's switches. Each withholds one capability from the hello, or
// makes the program answer like one that is wrong in one way.
const (
	shimNoWake      = "RW_SHIM_NO_WAKE"
	shimNoBoundary  = "RW_SHIM_NO_BOUNDARY"
	shimNoTelemetry = "RW_SHIM_NO_TELEMETRY"
	shimNoControl   = "RW_SHIM_NO_CONTROL"
	// shimFailProbe names a capability the program serves and whose probe it
	// fails.
	shimFailProbe = "RW_SHIM_FAIL_PROBE"
	// shimDropAfter closes the socket after that many turns.
	shimDropAfter = "RW_SHIM_DROP_AFTER"
	// shimHelperRequests sends the hello from a child process, which the
	// adapter's peer check must refuse.
	shimHelperRequests = "RW_SHIM_HELPER_REQUESTS"
	// shimNoHold reports ends that cannot be held: the adapter publishes them.
	shimNoHold = "RW_SHIM_NO_HOLD"
	// shimDropAnswer loses the core's answer to the first end once and sends
	// the same end again, as a harness whose answer was lost would.
	shimDropAnswer = "RW_SHIM_DROP_ANSWER"
	// shimFixtureVersion is what --version answers, for the refusals by
	// version; "none" answers nothing readable.
	shimFixtureVersion = "RW_SHIM_FIXTURE_VERSION"
	// fixtureHelperEnv marks the child shimHelperRequests starts.
	fixtureHelperEnv = "RW_SHIM_FIXTURE_HELPER"
)

// fixtureThread is the conversation the program reports.
const fixtureThread = "fixture-thread-0001"

// fixtureServed are the capabilities the program can serve, by the names its
// hello gives them.
var fixtureServed = []struct{ name, withheld string }{
	{"wake", shimNoWake},
	{"turn-boundary", shimNoBoundary},
	{"telemetry", shimNoTelemetry},
	{"control", shimNoControl},
	{"tool-transport", shimNoTool},
}

// fixtureFrame is one line of the exchange.
type fixtureFrame struct {
	Op         string          `json:"op"`
	ID         int64           `json:"id,omitempty"`
	Version    string          `json:"version,omitempty"`
	PID        int             `json:"pid,omitempty"`
	Thread     string          `json:"thread,omitempty"`
	Serves     []string        `json:"serves,omitempty"`
	Capability string          `json:"capability,omitempty"`
	OK         bool            `json:"ok,omitempty"`
	Error      string          `json:"error,omitempty"`
	State      string          `json:"state,omitempty"`
	Turn       string          `json:"turn,omitempty"`
	End        string          `json:"end,omitempty"`
	Outcome    string          `json:"outcome,omitempty"`
	Text       string          `json:"text,omitempty"`
	Hold       bool            `json:"hold,omitempty"`
	Reason     string          `json:"reason,omitempty"`
	Letter     string          `json:"letter,omitempty"`
	Notice     string          `json:"notice,omitempty"`
	Members    json.RawMessage `json:"members,omitempty"`
	Steered    bool            `json:"steered,omitempty"`
	// At is the program's own time of a turn's start, on the boot clock.
	At int64 `json:"at,omitempty"`
	// The tool transport's fields: the probe's offer, what the program
	// registered, and a call's report and result.
	Tools     []bridge.ToolDescriptor `json:"tools,omitempty"`
	Endpoint  string                  `json:"endpoint,omitempty"`
	Names     []string                `json:"names,omitempty"`
	Digest    string                  `json:"digest,omitempty"`
	Call      string                  `json:"call,omitempty"`
	Tool      string                  `json:"tool,omitempty"`
	Arguments json.RawMessage         `json:"arguments,omitempty"`
	Texts     []string                `json:"texts,omitempty"`
	IsError   bool                    `json:"isError,omitempty"`
}

// fixtureLaunch is what the program was started with.
type fixtureLaunch struct {
	session, room, role, brief, connect string
	rest                                []string
}

// runFixture is the entry point when this binary is re-executed as `fixture`.
func runFixture(args []string) int {
	args = harnessArgs(args)
	if slices.Contains(args, "--version") {
		version := os.Getenv(shimFixtureVersion)
		switch version {
		case "":
			fmt.Println("fixture 1.0.0")
		case "none":
			fmt.Println("fixture, version unknown")
		default:
			fmt.Println("fixture " + version)
		}
		return 0
	}
	if os.Getenv(fixtureHelperEnv) != "" {
		return fixtureHelper(args)
	}
	launch, err := parseFixtureLaunch(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fixture: %v\n", err)
		if mark := os.Getenv(shimReadyFile); mark != "" {
			_ = os.WriteFile(mark+".refused", []byte(err.Error()+"\n"), 0o600)
		}
		return 2
	}
	if err := insideACase(); err != nil {
		fmt.Fprintf(os.Stderr, "fixture: %v\n", err)
		return 1
	}
	if launch.connect != "" {
		recordShimCwd("fixture")
		return fixtureSessionMain(launch)
	}
	return fixtureTerminal()
}

// parseFixtureLaunch parses strictly: each flag once, nothing it does not
// know before the --, and the session's description complete. A flag the
// host should not pass fails the launch here.
func parseFixtureLaunch(args []string) (fixtureLaunch, error) {
	var launch fixtureLaunch
	values := map[string]*string{
		"--session": &launch.session, "--room": &launch.room, "--role": &launch.role,
		"--brief": &launch.brief, "--connect": &launch.connect,
	}
	seen := map[string]bool{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			launch.rest = args[i+1:]
			break
		}
		target, known := values[arg]
		if !known {
			return launch, fmt.Errorf("unknown argument %q", arg)
		}
		if seen[arg] {
			return launch, fmt.Errorf("%s given twice", arg)
		}
		if i+1 >= len(args) {
			return launch, fmt.Errorf("%s needs a value", arg)
		}
		seen[arg] = true
		*target = args[i+1]
		i++
	}
	if launch.session == "" || launch.room == "" || launch.role == "" {
		return launch, errors.New("the launch must name --session, --room and --role")
	}
	if launch.connect == "" && launch.brief != "" {
		return launch, errors.New("--brief goes to the session half, never to the terminal")
	}
	if launch.connect != "" && launch.rest != nil {
		return launch, errors.New("the session half takes no arguments of the caller's")
	}
	return launch, nil
}

// fixtureTerminal is the foreground half: the session's own sends and
// requests, and its stay until it is asked to stop.
func fixtureTerminal() int {
	// The session half answered its probes before the wrapper started this
	// half, so a sender may go now; mail sent earlier waits in the mailbox.
	if mark := os.Getenv(shimReadyFile); mark != "" {
		_ = os.WriteFile(mark, []byte("fixture"), 0o600)
	}
	go serveRequests()
	if code := sendAsAsked(); code != 0 {
		return code
	}
	return shimReportState()
}

// fixtureHelper is the child shimHelperRequests starts: it says hello in its
// parent's name from a process the adapter did not start.
func fixtureHelper(args []string) int {
	launch, err := parseFixtureLaunch(args)
	if err != nil {
		return 2
	}
	conn, err := net.Dial("unix", launch.connect)
	if err != nil {
		return 3
	}
	defer func() { _ = conn.Close() }()
	parent, _ := strconv.Atoi(os.Getenv(fixtureHelperEnv))
	hello, _ := json.Marshal(fixtureFrame{Op: "hello", Version: "1.0.0", PID: parent, Thread: fixtureThread, Serves: []string{"wake"}})
	_, _ = conn.Write(append(hello, '\n'))
	_, _ = bufio.NewReader(conn).ReadString('\n')
	return 0
}

// fixtureSession is the session half.
type fixtureSession struct {
	*shimSession
	conn    net.Conn
	writeMu sync.Mutex
	mu      sync.Mutex
	next    int64
	waiting map[int64]chan fixtureFrame
	// turns counts the turns worked, for shimDropAfter.
	turns int
	tools fixtureTools
}

func fixtureSessionStart(launch fixtureLaunch) (*fixtureSession, error) {
	if os.Getenv(shimHelperRequests) != "" {
		child := exec.Command(os.Args[0], os.Args[1:]...)
		child.Env = append(os.Environ(), fixtureHelperEnv+"="+strconv.Itoa(os.Getpid()))
		if err := child.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "fixture: the helper: %v\n", err)
		}
		return nil, errors.New("the hello went from a helper")
	}
	conn, err := net.Dial("unix", launch.connect)
	if err != nil {
		return nil, err
	}
	s := &fixtureSession{shimSession: &shimSession{thread: fixtureThread}, conn: conn, waiting: map[int64]chan fixtureFrame{}}
	var serves []string
	for _, capability := range fixtureServed {
		if os.Getenv(capability.withheld) == "" {
			serves = append(serves, capability.name)
		}
	}
	if err := s.send(fixtureFrame{Op: "hello", Version: "1.0.0", PID: os.Getpid(), Thread: fixtureThread, Serves: serves}); err != nil {
		return nil, err
	}
	return s, nil
}

func fixtureSessionMain(launch fixtureLaunch) int {
	s, err := fixtureSessionStart(launch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fixture: %v\n", err)
		// Stays up as a session would whose transport failed: the wrapper
		// ends it with the run.
		time.Sleep(requestLifetime)
		return 0
	}
	s.read()
	// The connection is gone: the session stays until the run ends it, so a
	// dropped socket is a withdrawal and not a departure.
	time.Sleep(requestLifetime)
	return 0
}

func (s *fixtureSession) send(frame fixtureFrame) error {
	raw, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err = s.conn.Write(append(raw, '\n'))
	return err
}

// ask sends a request and waits for its answer.
func (s *fixtureSession) ask(frame fixtureFrame, bound time.Duration) (fixtureFrame, error) {
	s.mu.Lock()
	s.next++
	frame.ID = s.next
	answer := make(chan fixtureFrame, 1)
	s.waiting[frame.ID] = answer
	s.mu.Unlock()
	if err := s.send(frame); err != nil {
		return fixtureFrame{}, err
	}
	select {
	case got := <-answer:
		return got, nil
	case <-time.After(bound):
		return fixtureFrame{}, fmt.Errorf("no answer to %s within %s", frame.Op, bound)
	}
}

// read takes the adapter's frames until the connection ends.
func (s *fixtureSession) read() {
	reader := bufio.NewReaderSize(s.conn, 1<<16)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		var frame fixtureFrame
		if json.Unmarshal(line, &frame) != nil {
			return
		}
		if frame.Op == "answer" {
			s.mu.Lock()
			waiter := s.waiting[frame.ID]
			delete(s.waiting, frame.ID)
			s.mu.Unlock()
			if waiter != nil {
				waiter <- frame
			}
			continue
		}
		go s.serve(frame)
	}
}

// serve answers one request of the adapter.
func (s *fixtureSession) serve(frame fixtureFrame) {
	reply := fixtureFrame{Op: "answer", ID: frame.ID}
	switch frame.Op {
	case "probe":
		reply = s.probe(frame, reply)
	case "reserve":
		reply.OK, reply.Thread = true, s.thread
	case "deliver":
		reply = s.delivered(frame, reply)
	case "release":
		return
	default:
		reply.Error = "the fixture does not take " + frame.Op
	}
	_ = s.send(reply)
}

// probe answers without acting: an acknowledgment, the turn state, the last
// sample, ready.
func (s *fixtureSession) probe(frame, reply fixtureFrame) fixtureFrame {
	if os.Getenv(shimFailProbe) == frame.Capability {
		reply.Error = "the probe was made to fail"
		return reply
	}
	reply.OK = true
	switch frame.Capability {
	case "turn-boundary":
		s.turn.mu.Lock()
		reply.State, reply.Turn = "idle", ""
		if s.turn.open != "" {
			reply.State, reply.Turn = "turn", s.turn.open
		}
		s.turn.mu.Unlock()
	case "telemetry":
		reply.State = "none"
	case "control":
		reply.State = "ready"
	case "tool-transport":
		reply = s.registerTools(frame, reply)
	}
	return reply
}
