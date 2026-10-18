//go:build rewaketest

package decisionrig

// The program the fixture adapter runs in the rig, as the harness of every
// session a test launches. The backend starts its server half with --connect:
// it says hello, answers the probes of what it serves, and on command
// reports a compaction or drops its link and connects again. The wrapper
// starts its terminal half, the session's foreground, which says who it runs
// as and exits on command. Each half starts askers on command, so a question
// comes from a process below the wrapper either way — and from the server
// half while the foreground has already ended.
//
// The adapter's exchange is spelled out here rather than imported, as the
// other rigs' programs do: a program sharing the adapter's types would agree
// with it by construction.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// programEnv is the program's control directory; it marks this test binary
// started as the program.
const programEnv = "DECISIONRIG_PROGRAM"

// The halves' directories under the control directory.
const (
	serverHalf   = "server"
	terminalHalf = "terminal"
)

// The commands the halves take.
const (
	opAsk     = "ask"
	opCompact = "compact"
	opDrop    = "drop"
	opExit    = "exit"
)

// command is one command of the test to a half: Out is where its result goes.
type command struct {
	Op  string `json:"op"`
	Out string `json:"out"`
	Job job    `json:"job"`
}

// frame is one line of the adapter's exchange, as far as the program speaks it.
type frame struct {
	Op         string   `json:"op"`
	ID         int64    `json:"id,omitempty"`
	Version    string   `json:"version,omitempty"`
	PID        int      `json:"pid,omitempty"`
	Thread     string   `json:"thread,omitempty"`
	Serves     []string `json:"serves,omitempty"`
	Capability string   `json:"capability,omitempty"`
	OK         bool     `json:"ok,omitempty"`
	State      string   `json:"state,omitempty"`
	Outcome    string   `json:"outcome,omitempty"`
}

// served are the capabilities the program serves and its probe answers:
// Control, so a compaction is counted, Telemetry, so it is published, and the
// turn boundary, without which a worker is not started; it never has a turn.
var served = map[string]string{"control": "ready", "telemetry": "idle", "turn-boundary": "idle"}

func runProgram(ctl string, args []string) int {
	if slices.Contains(args, "--version") {
		fmt.Println("fixture 1.0.0")
		return 0
	}
	if index := slices.Index(args, "--connect"); index >= 0 && index+1 < len(args) {
		return runServer(ctl, args[index+1])
	}
	return runTerminal(ctl)
}

func runTerminal(ctl string) int {
	ready := map[string]any{"session": os.Getenv("REWAKE_SESSION"), "epoch": os.Getenv("REWAKE_EPOCH"), "pid": os.Getpid()}
	if err := writeJSON(filepath.Join(ctl, terminalHalf, "ready"), ready); err != nil {
		fmt.Fprintln(os.Stderr, "decisionrig terminal:", err)
		return 1
	}
	exit := make(chan struct{})
	go serveCommands(filepath.Join(ctl, terminalHalf), func(c command) {
		switch c.Op {
		case opAsk:
			startAsker(c)
		case opExit:
			_ = writeJSON(c.Out, result{Kind: "done"})
			close(exit)
		}
	})
	<-exit
	return 0
}

// link is the server half's connection to the adapter.
type link struct {
	socket  string
	mu      sync.Mutex
	conn    net.Conn
	links   int
	probed  map[string]bool
	dropped bool
}

func runServer(ctl, socket string) int {
	l := &link{socket: socket}
	if err := l.connect(ctl); err != nil {
		fmt.Fprintln(os.Stderr, "decisionrig server:", err)
		return 1
	}
	serveCommands(filepath.Join(ctl, serverHalf), func(c command) {
		switch c.Op {
		case opAsk:
			startAsker(c)
		case opCompact:
			err := l.send(frame{Op: "compacted", Outcome: "done"})
			_ = writeJSON(c.Out, resultOf(err))
		case opDrop:
			l.mu.Lock()
			l.dropped = true
			_ = l.conn.Close()
			l.mu.Unlock()
			// The adapter takes one link at a time and refuses a hello
			// while it still holds the old one: it notices the close first.
			time.Sleep(300 * time.Millisecond)
			_ = writeJSON(c.Out, resultOf(l.connect(ctl)))
		}
	})
	return 0
}

// connect dials the adapter and says hello; the reader answers the probes
// and records each link whose probes were all answered.
func (l *link) connect(ctl string) error {
	conn, err := net.Dial("unix", l.socket)
	if err != nil {
		return err
	}
	l.mu.Lock()
	l.conn, l.dropped, l.probed = conn, false, map[string]bool{}
	l.mu.Unlock()
	serves := make([]string, 0, len(served))
	for capability := range served {
		serves = append(serves, capability)
	}
	sort.Strings(serves)
	go l.read(ctl, conn)
	return l.send(frame{Op: "hello", Version: "1.0.0", PID: os.Getpid(), Thread: "decisionrig-thread", Serves: serves})
}

func (l *link) read(ctl string, conn net.Conn) {
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			l.mu.Lock()
			dropped := l.dropped
			l.mu.Unlock()
			if !dropped {
				// The adapter closed the link: the session is ending.
				os.Exit(0)
			}
			return
		}
		var f frame
		if json.Unmarshal(line, &f) != nil || f.ID == 0 || f.Op == "answer" {
			continue
		}
		answer := frame{Op: "answer", ID: f.ID, OK: true}
		if f.Op == "probe" {
			answer.State = served[f.Capability]
		}
		_ = l.send(answer)
		if f.Op == "probe" {
			l.mu.Lock()
			l.probed[f.Capability] = true
			if len(l.probed) == len(served) {
				l.links++
				_ = writeJSON(filepath.Join(ctl, serverHalf, "links"), l.links)
			}
			l.mu.Unlock()
		}
	}
}

func (l *link) send(f frame) error {
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err = l.conn.Write(append(raw, '\n'))
	return err
}

// serveCommands takes each command file of dir once, in order, each in a
// goroutine of its own, so an asker held at a pause holds no other command.
func serveCommands(dir string, handle func(command)) {
	for {
		names, _ := filepath.Glob(filepath.Join(dir, "*.cmd"))
		sort.Strings(names)
		for _, name := range names {
			taken := strings.TrimSuffix(name, ".cmd") + ".taken"
			if os.Rename(name, taken) != nil {
				continue
			}
			var c command
			if err := readJSON(taken, &c); err != nil {
				continue
			}
			go handle(c)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// startAsker starts the asker a command describes below this half.
func startAsker(c command) {
	j := c.Job
	j.Out = c.Out
	path := c.Out + ".job"
	if err := writeJSON(path, j); err != nil {
		_ = writeJSON(c.Out, resultOf(err))
		return
	}
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(j.environment(os.Environ()), askerEnv+"="+path)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: j.Detach == detachSetsid}
	if j.Namespace {
		cmd.SysProcAttr.Cloneflags = syscall.CLONE_NEWUSER
		cmd.SysProcAttr.UidMappings = []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getuid(), Size: 1}}
		cmd.SysProcAttr.GidMappings = []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getgid(), Size: 1}}
	}
	if err := cmd.Start(); err != nil {
		_ = writeJSON(c.Out, result{Kind: kindUnstarted, Error: err.Error()})
		return
	}
	_ = cmd.Wait()
}
