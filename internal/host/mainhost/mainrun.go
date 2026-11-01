package mainhost

import (
	"bytes"
	"errors"
	"net"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/host/stream"
	"github.com/praline-labs/rewake/internal/host/terminal"
	"github.com/praline-labs/rewake/internal/infra/build"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// ClosingMark is the private sequence main's wrapper writes on its terminal
// when the harness says it has begun to end its process: the shell's scanner
// reads it in main's stream and starts its closing screen, or keeps the one
// main's leave of its screen began (docs/shell.md); the host reads it so no
// restart begins while main ends. The OSC number is one no terminal or
// program is known to use; shell/src/scan.ts reads the same bytes.
const ClosingMark = "\x1b]6262;rewake-closing\x07"

// The closing's bounds, as the shell had them: main hears its terminal hang
// up, has hangupWait to end, then SIGTERM to its group and termWait, then
// SIGKILL. Variables, so a test of the host's end need not wait them out.
var (
	hangupWait = 5 * time.Second
	termWait   = 3 * time.Second
)

// registerPoll is how often the host looks for main's session.
const registerPoll = 100 * time.Millisecond

// mainRun is main's wrapper on the host's terminal.
type mainRun struct {
	cmd   *exec.Cmd
	pty   *terminal.Plain
	relay *stream.Relay
	done  chan struct{}

	// The loop's.
	exited bool

	mu       sync.Mutex
	lease    *build.Lease
	listener *state.Listener
}

// HostSocket is the room's door to main's host: main's raw stream and
// snapshot, and the control mode (docs/shell.md#the-hosts-channel).
func HostSocket(dir string) string { return state.SocketPath(dir, "host", "") }

// modeControl is the host socket's mode for a client of the channel.
const modeControl byte = 'c'

// startMain starts main's wrapper on a terminal the host owns, at the first
// shell's size, with the person's colors; true ends the host.
func (h *Host) startMain() bool {
	size := h.size
	if size.Cols < 20 || size.Rows < 5 {
		size = fallbackSize
	}
	pty, err := terminal.OpenPlain(size)
	if err != nil {
		return h.failStart(err)
	}
	grid, err := terminal.NewGrid(size)
	if err != nil {
		_ = pty.Close()
		return h.failStart(err)
	}
	grid.SetDefaults(terminal.ParseDefaults(h.colors))
	m := &mainRun{pty: pty, done: make(chan struct{})}
	m.relay = stream.New(pty, grid, h.colors, markWatcher(func() { h.post(ev{kind: kMark}) }))
	cmd := exec.Command(h.cfg.Main[0], h.cfg.Main[1:]...)
	env := withoutEnv(os.Environ(), HostFDEnv, terminal.DefaultsEnv)
	env = append(env, h.cfg.MainEnv...)
	if h.colors != "" {
		env = append(env, terminal.DefaultsEnv+"="+h.colors)
	}
	cmd.Env = env
	if err = pty.Prepare(cmd); err == nil {
		err = cmd.Start()
	}
	_ = pty.ReleaseSide()
	if cmd.Process == nil {
		_ = pty.Close()
		_ = grid.Close()
		return h.failStart(err)
	}
	m.cmd = cmd
	h.main = m
	go m.relay.Pump()
	go m.relay.Answer()
	go func() {
		err := cmd.Wait()
		close(m.done)
		h.post(ev{kind: kMainExited, code: exitCode(err)})
	}()
	go h.serveMain(m)
	return false
}

// failStart ends the host when main could not start.
func (h *Host) failStart(err error) bool {
	_, _ = os.Stderr.WriteString("rewake: main could not start on the host's terminal: " + err.Error() + "\n")
	h.code = 1
	return h.close(false)
}

// serveMain waits for main's session to be registered, which proves the
// room made and this host's main its main, then takes the room's lease and
// serves main's stream and the door on the host socket.
func (h *Host) serveMain(m *mainRun) {
	pid := m.cmd.Process.Pid
	tick := time.NewTicker(registerPoll)
	defer tick.Stop()
	for !registered(h.cfg.Dir, pid) {
		select {
		case <-m.done:
			return
		case <-tick.C:
		}
	}
	lease, err := build.Acquire(h.cfg.Base, h.cfg.Room)
	if err != nil {
		_, _ = os.Stderr.WriteString("rewake: main's stream could not be served: " + err.Error() + "\n")
		return
	}
	path := HostSocket(h.cfg.Dir)
	// A host that died left its socket; one main per room makes it a dead one.
	_ = state.Discard(path)
	listener, err := state.Listen(path)
	if err != nil {
		lease.Release()
		_, _ = os.Stderr.WriteString("rewake: main's stream could not be served: " + err.Error() + "\n")
		return
	}
	m.mu.Lock()
	m.lease, m.listener = lease, listener
	m.mu.Unlock()
	go m.relay.Serve(listener, map[byte]func(net.Conn){modeControl: h.door})
	h.post(ev{kind: kRegistered, ref: &MainRef{PID: pid, Socket: path}})
}

// registered says main's session is in the room, served by the wrapper the
// host started.
func registered(dir string, pid int) bool {
	sessions, err := registry.ListReadOnly(dir)
	if err != nil {
		return false
	}
	start, err := proc.StartTime(pid)
	if err != nil {
		return false
	}
	for _, s := range sessions {
		if s.Role == role.Main.ID && s.ServicePID == pid && s.ServiceStart == start {
			return true
		}
	}
	return false
}

// end hangs main up, then SIGTERM and SIGKILL to its group within the
// bounds; its exit reaches the loop from its waiter.
func (m *mainRun) end() {
	_ = m.pty.Close()
	if m.waitDone(hangupWait) {
		return
	}
	_ = syscall.Kill(-m.cmd.Process.Pid, syscall.SIGTERM)
	if m.waitDone(termWait) {
		return
	}
	_ = syscall.Kill(-m.cmd.Process.Pid, syscall.SIGKILL)
}

func (m *mainRun) waitDone(d time.Duration) bool {
	select {
	case <-m.done:
		return true
	case <-time.After(d):
		return false
	}
}

// close lets go of the room: the door, then the lease.
func (m *mainRun) close() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listener != nil {
		_ = m.listener.Close()
	}
	if m.lease != nil {
		m.lease.Release()
	}
	_ = m.pty.Close()
	_ = m.relay.Grid().Close()
}

// markWatcher calls seen once main's closing mark goes by, a mark cut
// between two reads included.
func markWatcher(seen func()) func([]byte) {
	mark := []byte(ClosingMark)
	keep := len(mark) - 1
	var carry []byte
	once := false
	return func(p []byte) {
		if once {
			return
		}
		head := p[:min(len(p), keep)]
		if bytes.Contains(p, mark) || len(carry) > 0 && bytes.Contains(append(carry, head...), mark) {
			once = true
			seen()
			return
		}
		if len(p) >= keep {
			carry = append(carry[:0], p[len(p)-keep:]...)
			return
		}
		carry = append(carry, p...)
		if len(carry) > keep {
			carry = append(carry[:0], carry[len(carry)-keep:]...)
		}
	}
}

// exitCode is a wrapper's exit as a code: a signal's end is 1.
func exitCode(err error) int {
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &exit) && exit.ExitCode() >= 0:
		return exit.ExitCode()
	}
	return 1
}
