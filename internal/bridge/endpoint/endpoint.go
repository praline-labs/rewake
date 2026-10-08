package endpoint

import (
	"bufio"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/proc"
)

// The bounds of docs/mail-bridge-turns.md#waits-and-their-bounds.
const (
	observationWait = 2 * time.Second
	// maxOthers bounds the connections besides the servers'; maxServers the
	// servers', which a harness restarts and which live as long as it does.
	maxOthers  = 16
	maxServers = 4
	// shortExchange bounds a connection that is not a server's.
	shortExchange = 5 * time.Second
)

// Endpoint serves one run's context socket.
type Endpoint struct {
	cfg      Config
	listener *net.UnixListener
	path     string
	calls    *calls
	acks     sync.WaitGroup
	// running are the transport's calls under way; Close lets them answer.
	running sync.WaitGroup

	mu    sync.Mutex
	roots []int
	// readsOff says why the run's tool may not read, as the backend's
	// checks find it; nil until the backend is known.
	readsOff func() string
	servers  int
	others   int
	// generation numbers the server connections, counted up per run;
	// bound marks those whose thread is told, primary names the thread the
	// gateway holds (channel.go).
	generation uint64
	bound      map[uint64]bool
	primary    func() string
	// conns are the connections open now; true marks a transport's call.
	conns map[*net.UnixConn]bool
	// transport is the process a transport's calls come from, childEnv
	// their children's environment, children the children running now and
	// slots the calls running at once (transport.go, run.go).
	transport transportPeer
	childEnv  []string
	children  map[int]uint64
	slots     chan struct{}
	closed    bool
	done      chan struct{}
	once      sync.Once
}

// Listen binds the run's context socket at path, mode 0600, replacing a file
// a dead run of the same name left there.
func Listen(path string, cfg Config) (*Endpoint, error) {
	if cfg.Wait == 0 {
		cfg.Wait = observationWait
	}
	if cfg.SameBuild == nil {
		cfg.SameBuild = sameBuild
	}
	if cfg.Descends == nil {
		cfg.Descends = proc.Default.Descends
	}
	if cfg.Gate == nil {
		cfg.Gate = NewGate(nil)
	}
	_ = os.Remove(path)
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	listener.SetUnlinkOnClose(true)
	_ = os.Chmod(path, 0o600)
	e := &Endpoint{cfg: cfg, listener: listener, path: path, calls: newCalls(), conns: map[*net.UnixConn]bool{}, slots: make(chan struct{}, maxChildren), done: make(chan struct{})}
	go e.accept()
	return e, nil
}

// SetRoots names the processes this wrapper started that may run the tool:
// the harness, and a backend's own server. Until it is called no hello is
// accepted, since no peer can be shown to descend from the harness.
func (e *Endpoint) SetRoots(pids ...int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.roots = append([]int(nil), pids...)
}

// SetReadsOff names what says whether the run's tool may read: the
// backend's own checks, known only once the plan is made.
func (e *Endpoint) SetReadsOff(readsOff func() string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.readsOff = readsOff
}

// readsOffNow is why the tool may not read now, or "".
func (e *Endpoint) readsOffNow() string {
	e.mu.Lock()
	readsOff := e.readsOff
	e.mu.Unlock()
	if readsOff == nil {
		return ""
	}
	return readsOff()
}

// Gate is the gate the endpoint's acknowledgments enter.
func (e *Endpoint) Gate() *Gate { return e.cfg.Gate }

// Close stops serving, closes every connection but those of the transport's
// calls under way, and waits for those calls and for the acknowledgments
// under way. A call finishes and is answered as at any other time, except
// that a child its kill did not reach is waited for only reapWait more
// (run.go); an acknowledgment is bounded before its check and runs its writes
// to their end.
func (e *Endpoint) Close() {
	e.once.Do(func() {
		e.mu.Lock()
		e.closed = true
		for conn, call := range e.conns {
			if !call {
				_ = conn.Close()
			}
		}
		e.mu.Unlock()
		close(e.done)
		_ = e.listener.Close()
	})
	e.running.Wait()
	e.acks.Wait()
}

func (e *Endpoint) accept() {
	for {
		conn, err := e.listener.AcceptUnix()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			select {
			case <-e.done:
				return
			default:
				continue
			}
		}
		go e.serve(conn)
	}
}

// serve reads a connection's hello, checks the peer, and answers its
// requests, each on its own, until it closes.
func (e *Endpoint) serve(conn *net.UnixConn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(shortExchange))
	reader := bufio.NewReaderSize(conn, 64<<10)
	var greeting hello
	line, err := readLine(reader)
	if err != nil || json.Unmarshal(line, &greeting) != nil {
		return
	}
	writer := &lineWriter{conn: conn}
	release, err := e.admit(conn, greeting)
	if err != nil {
		if greeting.Role == roleServer {
			e.refused(conn)
		}
		writer.write(response{Error: err.Error()})
		return
	}
	defer release()
	writer.write(response{})
	if greeting.Role == roleTransport {
		if peer, err := peerOf(conn); err == nil {
			e.serveCall(conn, reader, writer, int(peer.Pid))
		}
		return
	}
	var generation uint64
	if greeting.Role == roleServer {
		// A server's connection lasts as long as it does: its close is how
		// the wrapper learns the server is gone.
		_ = conn.SetDeadline(time.Time{})
		var closed func()
		generation, closed = e.connected()
		defer closed()
	}
	var running sync.WaitGroup
	defer running.Wait()
	for {
		line, err := readLine(reader)
		if err != nil {
			return
		}
		var asked request
		if json.Unmarshal(line, &asked) != nil {
			return
		}
		running.Add(1)
		go func() {
			defer running.Done()
			writer.write(e.answer(conn, greeting.Role, asked, generation))
		}()
	}
}

// admit checks a hello: the peer is this user, runs below a process this
// wrapper started, is this build, and — a server or a child — carries this
// launch's capability. It also holds the peer's place among the bounded
// connections.
func (e *Endpoint) admit(conn *net.UnixConn, greeting hello) (func(), error) {
	peer, err := peerOf(conn)
	if err != nil {
		return nil, fmt.Errorf("the peer cannot be identified: %v", err)
	}
	if int(peer.Uid) != os.Getuid() {
		return nil, errors.New("the peer runs as another user")
	}
	switch greeting.Role {
	case roleServer, roleChild:
		if subtle.ConstantTimeCompare([]byte(greeting.Capability), []byte(e.cfg.Capability)) != 1 || e.cfg.Capability == "" {
			return nil, errors.New("the capability is not this launch's")
		}
	case roleHook:
	case roleTransport:
		// The harness's own process, exactly: not a build of rewake, and
		// never something running below it.
		if err := e.fromTransport(int(peer.Pid)); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("a role this endpoint does not serve: %q", greeting.Role)
	}
	if greeting.Role != roleTransport {
		e.mu.Lock()
		roots := e.roots
		e.mu.Unlock()
		// A transport's call runs below the wrapper itself, in a child the
		// endpoint started and knows by its pid.
		if greeting.Role != roleChild || !e.isOwnChild(int(peer.Pid)) {
			if err := e.below(int(peer.Pid), roots); err != nil {
				return nil, err
			}
		}
		if err := e.cfg.SameBuild(int(peer.Pid)); err != nil {
			return nil, fmt.Errorf("the peer is another build of rewake than this run's wrapper: %v", err)
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil, errors.New("the wrapper is closing")
	}
	count, limit := &e.others, maxOthers
	if greeting.Role == roleServer {
		count, limit = &e.servers, maxServers
	}
	if *count >= limit {
		return nil, errors.New("too many connections to this run's wrapper right now")
	}
	*count++
	call := greeting.Role == roleTransport
	e.conns[conn] = call
	if call {
		e.running.Add(1)
	}
	return func() {
		e.mu.Lock()
		*count--
		delete(e.conns, conn)
		e.mu.Unlock()
		if call {
			e.running.Done()
		}
	}, nil
}

func (e *Endpoint) below(pid int, roots []int) error {
	if len(roots) == 0 {
		return errors.New("the harness of this run has not started yet")
	}
	for _, root := range roots {
		if e.cfg.Descends(pid, root) == nil {
			return nil
		}
	}
	return fmt.Errorf("process %d does not run below the harness of this run", pid)
}

// answer serves one request of a connection whose role was admitted; a
// server's carries its connection's generation.
func (e *Endpoint) answer(conn *net.UnixConn, role string, asked request, generation uint64) response {
	reply := response{ID: asked.ID}
	var err error
	switch {
	case role == roleServer && asked.Op == opTicket && asked.Ask != nil:
		var ticket bridge.Ticket
		ticket, err = e.issue(*asked.Ask, generation)
		reply.Ticket = &ticket
	case role == roleChild && asked.Op == opConfirm && asked.Ticket != nil:
		err = e.confirm(conn, *asked.Ticket)
	case role == roleServer && asked.Op == opReport:
		err = e.report(asked.Payload, generation)
	case role == roleHook && asked.Op == opObserve:
		e.hookWith(asked.Payload, asked.Limits)
	default:
		err = fmt.Errorf("a %s may not ask %q", role, asked.Op)
	}
	if err != nil {
		return response{ID: asked.ID, Error: err.Error()}
	}
	return reply
}

// lineWriter writes whole lines, one at a time, to a connection that several
// requests answer on.
type lineWriter struct {
	mu   sync.Mutex
	conn *net.UnixConn
}

func (w *lineWriter) write(value any) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = w.conn.Write(append(encoded, '\n'))
}

// readLine reads one line within maxLine; a longer one ends the connection.
func readLine(reader *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		chunk, more, err := reader.ReadLine()
		if err != nil {
			return nil, err
		}
		line = append(line, chunk...)
		if len(line) > maxLine {
			return nil, errors.New("a line over its bound")
		}
		if !more {
			return line, nil
		}
	}
}

// peerOf reads the credentials of the process at the other end of a unix
// socket, as the kernel recorded them when it connected or listened.
func peerOf(conn *net.UnixConn) (*syscall.Ucred, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return nil, err
	}
	var cred *syscall.Ucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return nil, err
	}
	return cred, credErr
}

// sameBuild says whether a process runs the file this one runs: the same
// device and inode behind /proc/<pid>/exe. A file replaced on disk since is
// another inode, so a server or child started from a newer install is
// refused, however its version reads.
func sameBuild(pid int) error {
	own, err := os.Stat("/proc/self/exe")
	if err != nil {
		return err
	}
	theirs, err := os.Stat(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return err
	}
	if !os.SameFile(own, theirs) {
		return errors.New("its executable is another file")
	}
	return nil
}
