//go:build rewakefixture

package fixture

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/proc"
)

// readinessBound is how long the program has to say hello, and each probe to
// be answered. A variable so the package's tests can shorten it; a backend
// takes its value when made, so a test's change reaches no backend running.
var readinessBound = 8 * time.Second

// stopGrace is each stage of ending the program's group: SIGTERM, then
// SIGKILL this long after.
const stopGrace = 2 * time.Second

// backend runs the program and keeps what it has proven: the capabilities
// whose probes it answered on the connection it holds now, and nothing else.
type backend struct {
	program string
	args    []string
	env     []string
	dir     string
	socket  string
	epoch   string
	bound   time.Duration
	endWait time.Duration

	handler harness.CompletionHandler
	note    func(string)

	listener net.Listener
	process  *exec.Cmd
	pid      int
	start    uint64
	exited   chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc

	// greeted is closed once a hello from the program is taken, hello once
	// its probes have run too: the hello has the start's bound, and each probe
	// its own, so a probe that is never answered costs only its capability.
	greeted   chan struct{}
	greetOnce sync.Once
	hello     chan struct{}
	helloOnce sync.Once

	mu      sync.Mutex
	link    *link
	thread  string
	version string
	live    map[string]bool
	telem   telemetry
	turns   map[string]turnRecord
	ends    map[string]harness.Completion

	// closing stops new connections and new calls into the wrapper's
	// handler; Close then waits for the goroutines reading the program
	// (routines) and for every call into the handler under way (users),
	// because the wrapper unmaps the read clock those calls capture from once
	// Close returns. waiting holds the connections not yet past their hello.
	closing  bool
	waiting  map[net.Conn]bool
	routines sync.WaitGroup
	users    sync.WaitGroup

	stopOnce  sync.Once
	closeOnce sync.Once
}

func newBackend(program string, args, env []string, dir, socket, epoch string) *backend {
	return &backend{
		program: program, args: args, env: env, dir: dir, socket: socket, epoch: epoch, bound: readinessBound,
		endWait: endDeadline,
		exited:  make(chan struct{}), greeted: make(chan struct{}), hello: make(chan struct{}),
		live: map[string]bool{}, turns: map[string]turnRecord{}, ends: map[string]harness.Completion{},
		waiting: map[net.Conn]bool{},
	}
}

// Start listens, starts the program and returns once its hello and the probes
// of what it serves have run. A program that never says hello from its own
// process fails the start.
func (b *backend) Start(ctx context.Context, handler harness.CompletionHandler, note func(string)) error {
	b.handler, b.note = handler, note
	if b.note == nil {
		b.note = func(string) {}
	}
	b.ctx, b.cancel = context.WithCancel(ctx)
	_ = os.Remove(b.socket)
	listener, err := net.Listen("unix", b.socket)
	if err != nil {
		b.cancel()
		return err
	}
	_ = os.Chmod(b.socket, 0o600)
	b.listener = listener
	if err := b.spawn(); err != nil {
		_ = listener.Close()
		b.cancel()
		return err
	}
	b.routines.Add(1)
	go b.accept()
	timer := time.NewTimer(b.bound)
	defer timer.Stop()
	select {
	case <-b.greeted:
	case <-b.exited:
		b.Close()
		return errors.New("the fixture ended before its hello")
	case <-timer.C:
		b.Close()
		return fmt.Errorf("the fixture sent no hello from its own process within %s", b.bound)
	case <-ctx.Done():
		b.Close()
		return ctx.Err()
	}
	// The probes are bounded each, so this wait is too.
	select {
	case <-b.hello:
		return nil
	case <-b.exited:
		b.Close()
		return errors.New("the fixture ended before its probes were answered")
	case <-ctx.Done():
		b.Close()
		return ctx.Err()
	}
}

func (b *backend) spawn() error {
	command := exec.Command(b.program, b.args...)
	command.Env = b.env
	command.Dir = b.dir
	command.Stderr = os.Stderr
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	if err := command.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("%s is not installed or not in PATH", b.program)
		}
		return err
	}
	b.process, b.pid = command, command.Process.Pid
	start, err := proc.StartTime(b.pid)
	if err != nil {
		_ = syscall.Kill(-b.pid, syscall.SIGKILL)
		_ = command.Wait()
		return fmt.Errorf("the fixture's start could not be read: %w", err)
	}
	b.start = start
	go func() {
		_ = command.Wait()
		close(b.exited)
		b.withdraw(nil)
		b.cancel()
	}()
	return nil
}

// accept takes connections for as long as the backend runs. Each is checked
// against the process the backend started; a reconnect from it starts the
// exchange again.
func (b *backend) accept() {
	defer b.routines.Done()
	for {
		conn, err := b.listener.Accept()
		if err != nil {
			return
		}
		b.mu.Lock()
		if b.closing {
			b.mu.Unlock()
			_ = conn.Close()
			return
		}
		b.waiting[conn] = true
		b.mu.Unlock()
		b.routines.Add(1)
		go b.admit(conn)
	}
}

// admit runs one connection: the peer check, one hello, the probes, then
// whatever the program sends until it closes.
func (b *backend) admit(conn net.Conn) {
	defer b.routines.Done()
	defer func() {
		b.mu.Lock()
		delete(b.waiting, conn)
		b.mu.Unlock()
	}()
	unix, ok := conn.(*net.UnixConn)
	if !ok || !b.fromProgram(unix) {
		_ = conn.Close()
		return
	}
	reader := bufio.NewReader(conn)
	l := newLink(conn)
	_ = conn.SetReadDeadline(time.Now().Add(b.bound))
	first, err := l.read(reader)
	if err != nil || first.Op != opHello || first.PID != b.pid || first.Thread == "" {
		l.close()
		return
	}
	_ = conn.SetReadDeadline(time.Time{})
	b.mu.Lock()
	if b.link != nil || b.closing {
		// One hello per connection, one connection at a time: a second
		// connection while the first holds is not a reconnect.
		b.mu.Unlock()
		l.close()
		return
	}
	b.link, b.thread, b.version = l, first.Thread, first.Version
	b.live = map[string]bool{}
	b.mu.Unlock()
	b.greetOnce.Do(func() { close(b.greeted) })
	b.routines.Add(1)
	go func() {
		defer b.routines.Done()
		l.serve(reader, func(frame Frame) { b.handle(l, frame) })
	}()
	live := b.probe(l, first.Serves)
	b.mu.Lock()
	if b.link == l {
		b.live = live
	}
	b.mu.Unlock()
	b.helloOnce.Do(func() { close(b.hello) })
	<-l.closed
	b.withdraw(l)
}

// fromProgram is the peer check: the connecting process is the one the
// backend started, by pid and start time.
func (b *backend) fromProgram(conn *net.UnixConn) bool {
	raw, err := conn.SyscallConn()
	if err != nil {
		return false
	}
	var cred *syscall.Ucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil || credErr != nil {
		return false
	}
	if int(cred.Pid) != b.pid {
		return false
	}
	start, err := proc.StartTime(b.pid)
	return err == nil && start == b.start
}

// probe sends one probe per served capability, all at once, and answers the
// set whose probes came back right within the bound. A capability the hello
// did not name is not probed.
func (b *backend) probe(l *link, serves []string) map[string]bool {
	live := map[string]bool{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, capability := range Served {
		if !slices.Contains(serves, capability) {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			answer, err := l.ask(Frame{Op: opProbe, Capability: capability}, b.bound)
			if err == nil && probeAnswered(capability, answer) {
				mu.Lock()
				live[capability] = true
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return live
}

// probeAnswered says whether a probe's answer is the one its capability
// gives: an acknowledgment for Wake, a turn state for TurnBoundary, a sample
// or none for Telemetry, ready for Control.
func probeAnswered(capability string, answer Frame) bool {
	if !answer.OK {
		return false
	}
	switch capability {
	case TurnBoundary:
		return answer.State == "idle" || answer.Turn != "" && answer.State == "turn"
	case Telemetry:
		return answer.State == "none" || answer.State == "working" || answer.State == "idle"
	case Control:
		return answer.State == "ready"
	}
	return true
}

// Live answers the capabilities live now, for the package's tests and for
// the decisions below.
func (b *backend) Live() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []string
	for _, capability := range Served {
		if b.live[capability] {
			out = append(out, capability)
		}
	}
	return out
}

func (b *backend) isLive(capability string) (*link, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.link, b.link != nil && b.live[capability]
}

// Thread is the program's conversation while a connection with a live Wake
// holds.
func (b *backend) Thread() (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.link == nil || !b.live[Wake] {
		return "", inbox.ErrThreadUnavailable
	}
	return b.thread, nil
}

func (b *backend) Done() <-chan struct{} { return b.exited }

// ProcessID is the program's pid, zero before it started.
func (b *backend) ProcessID() int { return b.pid }
