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
	gen int
	why string
	cmd *exec.Cmd

	mu   sync.Mutex
	conn net.Conn
	err  bytes.Buffer

	// The loop's: what the shell has done so far.
	hello     bool
	given     bool // it was sent go
	ready     bool
	releasing bool // it was asked to release the terminal
	refused   string
	said      string // the last of close, lost, released

	// What it is, for the plan of an update: its digest or its file's,
	// closed digested once taken, and the contracts its hello said.
	digested     chan struct{}
	digest, file string
	requires     map[string]string
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
	p := &shellProc{gen: h.gen, why: why, conn: conn, digested: make(chan struct{})}
	cmd := exec.Command(h.cfg.Shell, h.cfg.ShellArgs...)
	cmd.Stdin, cmd.Stdout = os.Stdin, os.Stdout
	cmd.Stderr = (*tailWriter)(p)
	cmd.ExtraFiles = []*os.File{theirs}
	cmd.Env = append(withoutEnv(os.Environ(), HostFDEnv), append(h.cfg.ShellEnv, HostFDEnv+"=3")...)
	if err := cmd.Start(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("could not start the shell %s: %w", h.cfg.Shell, err)
	}
	p.cmd = cmd
	go p.takeDigest(h.cfg.Shell)
	// The exit reaches the loop after the last of what the shell said: a
	// shell writes released and exits at once, and its socket still holds the
	// state while the process is gone. The socket is read on for drainWithin
	// after the exit, as a child of the shell may hold its end open.
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		_ = conn.SetReadDeadline(time.Now().Add(drainWithin))
		close(exited)
	}()
	go func() {
		h.readShell(p)
		<-exited
		_ = conn.Close()
		h.post(ev{kind: kExit, gen: p.gen})
	}()
	h.after(helloWithin, ev{kind: kHelloLate, gen: p.gen})
	return p, nil
}

// readShell reads a shell's hello, then its messages, until its socket ends.
func (h *Host) readShell(p *shellProc) {
	l := newLines(p.conn)
	hello, err := takeHello(p.conn, l)
	if errors.Is(err, io.EOF) || errors.Is(err, os.ErrDeadlineExceeded) {
		// It ended before its hello: its exit says why.
		return
	}
	if err == nil && hello.Hello != clientShell {
		err = fmt.Errorf("a hello from %q on the shell's socket", hello.Hello)
	}
	if err != nil {
		h.post(ev{kind: kRefused, gen: p.gen, why: "the new shell was refused: " + err.Error()})
		return
	}
	if hello.Contracts != nil {
		p.mu.Lock()
		p.requires = hello.Contracts.Requires
		if p.requires == nil {
			p.requires = map[string]string{}
		}
		p.mu.Unlock()
	}
	h.post(ev{kind: kHello, gen: p.gen})
	for {
		line, err := l.next()
		if err != nil {
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
	if p != nil && p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
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

// tailWriter keeps the end of a shell's standard error.
type tailWriter shellProc

func (w *tailWriter) Write(b []byte) (int, error) {
	p := (*shellProc)(w)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.err.Write(b)
	if over := p.err.Len() - shellStderrKept; over > 0 {
		p.err.Next(over)
	}
	return len(b), nil
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
