package mainhost

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/infra/proc"
)

// HostFDEnv tells the shell which of its descriptors is its socket to the
// host: the third after the standard streams.
const HostFDEnv = "REWAKE_HOST_FD"

// shellStderrKept is how much of a shell's standard error is kept, for what
// a shell that did not start said.
const shellStderrKept = 4 << 10

// shellProc is one shell the host started: a generation of its own, its
// socket, and what it said and did.
type shellProc struct {
	gen   int
	why   string
	pid   int
	start uint64

	mu   sync.Mutex
	conn net.Conn
	err  bytes.Buffer
	// errPipe is the read end of its standard error, which a switch hands
	// over with the tail kept so far.
	errPipe *os.File

	// lines reads conn; exited closes once the shell has exited, errDone
	// once its standard error ended. stopping and paused, under mu, are a
	// switch's drop stopping the reader before its next post (drop.go).
	lines    *lines
	exited   chan struct{}
	errDone  chan struct{}
	stopping bool
	paused   chan struct{}

	// The loop's: what the shell has done so far.
	hello     bool
	given     bool // it was sent go
	ready     bool
	releasing bool // it was asked to release the terminal
	refused   string
	said      string // the last of close, lost, released
	reaped    bool   // its exit taken, its pid no longer its own

	// What it is, for the plan of an update: its digest or its file's,
	// closed digested once taken, and the contracts its hello said.
	digested     chan struct{}
	digest, file string
	requires     map[string]string
	face         *FaceContracts
}

// startShell starts the next generation of the shell on the person's
// terminal, with the host's socket as its descriptor 3. The shell touches no
// terminal before go; its hello, messages and exit come to the loop as
// events.
func (h *Host) startShell(why string) (*shellProc, error) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("could not open the shell's socket: %w", err)
	}
	ours, theirs := os.NewFile(uintptr(fds[0]), "host"), os.NewFile(uintptr(fds[1]), "shell")
	defer func() { _ = theirs.Close() }()
	conn, err := net.FileConn(ours)
	_ = ours.Close()
	if err != nil {
		return nil, fmt.Errorf("could not open the shell's socket: %w", err)
	}
	h.gen++
	p := &shellProc{gen: h.gen, why: why, conn: conn, digested: make(chan struct{}), errDone: make(chan struct{})}
	stderr, errEnd, err := os.Pipe()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("could not open the shell's standard error: %w", err)
	}
	cmd := exec.Command(h.cfg.Shell, h.cfg.ShellArgs...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, errEnd
	cmd.ExtraFiles = []*os.File{theirs}
	cmd.Env = append(withoutEnv(os.Environ(), HostFDEnv), append(h.cfg.ShellEnv, HostFDEnv+"=3")...)
	err = cmd.Start()
	_ = errEnd.Close()
	if err != nil {
		_ = conn.Close()
		_ = stderr.Close()
		return nil, fmt.Errorf("could not start the shell %s: %w", h.cfg.Shell, err)
	}
	p.pid, p.errPipe = cmd.Process.Pid, stderr
	p.start, _ = proc.StartTime(p.pid)
	// The loop reaps it (children.go), never the process handle.
	_ = cmd.Process.Release()
	go h.digest(p)
	go p.keepErr()
	h.follow(p, newLines(conn), true)
	h.after(helloWithin, ev{kind: kHelloLate, gen: p.gen})
	return p, nil
}

// follow waits for a shell's exit and reads it: its hello first when it is
// new. The exit reaches the loop after the last of what the shell said: a
// shell writes released and exits at once, and its socket still holds the
// state while the process is gone. The socket is read on for drainWithin
// after the exit, as a child of the shell may hold its end open.
func (h *Host) follow(p *shellProc, l *lines, fresh bool) {
	h.unreaped[p.gen] = p
	p.lines, p.exited = l, make(chan struct{})
	go func() {
		waitExit(p.pid)
		_ = p.conn.SetReadDeadline(time.Now().Add(drainWithin))
		close(p.exited)
	}()
	if fresh {
		go h.readShell(p)
	} else {
		go h.readMessages(p)
	}
}

// ended posts a shell's exit, once its socket is read to its end.
func (h *Host) ended(p *shellProc) {
	<-p.exited
	_ = p.conn.Close()
	// Its last words, unless a child of the shell holds its standard error.
	select {
	case <-p.errDone:
	case <-time.After(drainWithin):
	}
	h.post(ev{kind: kExit, gen: p.gen})
}

// readShell reads a shell's hello, then its messages, until its socket ends.
func (h *Host) readShell(p *shellProc) {
	hello, err := takeHello(p.conn, p.lines)
	if errors.Is(err, io.EOF) || errors.Is(err, os.ErrDeadlineExceeded) {
		// It ended before its hello: its exit says why.
		h.ended(p)
		return
	}
	if err == nil && hello.Hello != clientShell {
		err = fmt.Errorf("a hello from %q on the shell's socket", hello.Hello)
	}
	if err != nil {
		h.post(ev{kind: kRefused, gen: p.gen, why: "the new shell was refused: " + err.Error()})
		h.ended(p)
		return
	}
	p.took(hello)
	h.post(ev{kind: kHello, gen: p.gen})
	h.readMessages(p)
}

// readMessages reads a shell's messages until its socket ends, or until a
// switch's drop stops it between two posts, what it read of the next line
// kept.
func (h *Host) readMessages(p *shellProc) {
	for {
		line, err := p.lines.next()
		if errors.Is(err, os.ErrDeadlineExceeded) && !closed(p.exited) {
			p.mu.Lock()
			stopping := p.stopping
			if stopping {
				close(p.paused)
			}
			p.mu.Unlock()
			if stopping {
				return
			}
			// A drop's stop it had not met before the drop resumed.
			continue
		}
		if err != nil {
			h.ended(p)
			return
		}
		var msg Message
		if json.Unmarshal(line, &msg) != nil {
			p.send(Message{T: kindError, Why: fmt.Sprintf("a line that does not read: %.200q", line)})
			continue
		}
		h.post(ev{kind: kMsg, gen: p.gen, msg: msg})
	}
}

// send writes one message to the shell; a shell gone takes nothing.
func (p *shellProc) send(msg Message) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	_ = writeLine(p.conn, msg)
}

// kill ends the shell at once.
func (p *shellProc) kill() {
	if p != nil && p.pid > 0 && !p.reaped {
		_ = syscall.Kill(p.pid, syscall.SIGKILL)
	}
}

// lastWords are the last lines of the shell's standard error.
func (p *shellProc) lastWords() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	lines := strings.FieldsFunc(p.err.String(), func(c rune) bool { return c == '\n' || c == '\r' })
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[len(lines)-1])
}

// keepErr keeps the end of a shell's standard error until it ends.
func (p *shellProc) keepErr() {
	defer close(p.errDone)
	buf := make([]byte, 4096)
	for {
		n, err := p.errPipe.Read(buf)
		p.mu.Lock()
		p.err.Write(buf[:n])
		if over := p.err.Len() - shellStderrKept; over > 0 {
			p.err.Next(over)
		}
		p.mu.Unlock()
		if err != nil {
			// The loop closes the pipe once it reaps the shell: a switch may
			// be handing its descriptor over.
			return
		}
	}
}

// byGen is the shell of a generation the host still follows.
func (h *Host) byGen(gen int) *shellProc {
	switch {
	case h.shell != nil && h.shell.gen == gen:
		return h.shell
	case h.candidate != nil && h.candidate.gen == gen:
		return h.candidate
	}
	return nil
}

// withoutEnv is env less the named variables.
func withoutEnv(env []string, names ...string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		drop := false
		for _, n := range names {
			drop = drop || name == n
		}
		if !drop {
			out = append(out, kv)
		}
	}
	return out
}
