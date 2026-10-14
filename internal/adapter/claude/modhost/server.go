package modhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// Config is what a run's mod endpoint serves.
type Config struct {
	// Socket is the run's mod socket, in the room's sock/ directory.
	Socket string
	// Role is the run's role, as the role catalog names it, which a hello
	// is answered with.
	Role string
	// Record is where the last admitted hello is written for the run, empty
	// for nowhere: what a person or a live check reads of the mod's load.
	Record string
	// Wait bounds how long a request waits for the host to name the
	// harness's process; a request that comes before is held, then refused.
	Wait time.Duration
	// HostsSpaces is the run's role opening spaces, which a hello is
	// answered with: the mod then serves the spaces' side of the protocol.
	HostsSpaces bool
	// Admitted hears each admitted hello once it is bound and recorded,
	// before it is answered, so what the run proves by it is in the live
	// set by the time the mod acts on the answer; nil hears nothing.
	Admitted func(HelloRequest)
}

// Server is the wrapper's end of the mod's protocol for one run. Every
// request is checked by its peer on its own: the process at the other end of
// the connection must be the harness the host started, alive with its start
// time as the request arrives and again before the answer
// (docs/rules/space.md, SP4).
type Server struct {
	cfg Config

	mu      sync.Mutex
	harness adapter.RunProcess
	load    *HelloRequest
	known   chan struct{}
	named   sync.Once
	// closed stops admission and every write a handler has not begun; stop
	// is closed with it, so a request waiting for the harness gives up.
	closed bool
	stop   chan struct{}
	// workers counts each admitted handler among the run's workers, so the
	// room's lease outlasts a handler that outlasts Close.
	workers adapter.Workers
	// bindMu orders a hello's binding with its record: of two hellos at once,
	// the record holds the one Load holds.
	bindMu sync.Mutex

	listener *state.Listener
	http     *http.Server
	served   chan struct{}
	closing  sync.Once

	// answering runs between an answer's making and its writing, a seam for
	// the test of the second peer check.
	answering func()
}

// New is a run's mod endpoint, listening once Start binds it.
func New(cfg Config) *Server {
	return &Server{cfg: cfg, known: make(chan struct{}), stop: make(chan struct{}), served: make(chan struct{})}
}

// closeWait bounds how long Close waits for the handlers in flight. One that
// outlasts it, stalled in the filesystem say, is cut off from the socket and
// stays counted among the run's workers until it returns: the run's end holds
// the lease for it rather than the close holding the wrapper's exit.
const closeWait = time.Second

// CountWorkers counts every handler from its admission to its return among
// the run's workers; called before Start.
func (s *Server) CountWorkers(workers adapter.Workers) {
	s.mu.Lock()
	s.workers = workers
	s.mu.Unlock()
}

// peerKey carries the connection's peer through the request's context.
type peerKey struct{}

// peer is what the kernel recorded of the process that connected.
type peer struct {
	pid int
	err error
}

// Start binds the run's mod socket under the room's lease and serves it.
// Every request is a connection of its own: the peer is a property of the
// connection, so a connection kept alive across requests would carry one
// check over to the next.
func (s *Server) Start() error {
	if err := state.EnsureSubdir(filepath.Dir(s.cfg.Socket)); err != nil {
		return err
	}
	listener, err := state.Listen(s.cfg.Socket)
	if err != nil {
		return fmt.Errorf("could not listen on the mod socket %s: %w", s.cfg.Socket, err)
	}
	s.listener = listener
	s.http = &http.Server{
		Handler:           http.HandlerFunc(s.serve),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      (MaxWaitSeconds + 5) * time.Second,
		ConnContext: func(ctx context.Context, conn net.Conn) context.Context {
			return context.WithValue(ctx, peerKey{}, peerOf(conn))
		},
		// The wrapper's standard error is the harness's terminal: what the
		// server would log there is not for the person.
		ErrorLog: log.New(io.Discard, "", 0),
	}
	s.http.SetKeepAlivesEnabled(false)
	go func() {
		defer close(s.served)
		_ = s.http.Serve(listener)
	}()
	return nil
}

// HarnessStarted names the harness's process: from now on it alone is served.
func (s *Server) HarnessStarted(harness adapter.RunProcess) {
	s.mu.Lock()
	s.harness = harness
	s.mu.Unlock()
	s.named.Do(func() { close(s.known) })
}

// Load is the last admitted hello: a reload of the mod is a new load whose
// hello replaces the one before.
func (s *Server) Load() (HelloRequest, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.load == nil {
		return HelloRequest{}, false
	}
	return *s.load, true
}

// Close stops admission and every write not yet begun, waits for the
// handlers in flight within closeWait, then cuts what is left and removes the
// socket. A handler still running after it is counted among the run's
// workers (CountWorkers).
func (s *Server) Close() {
	s.closing.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		close(s.stop)
		if s.http == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), closeWait)
		defer cancel()
		if s.http.Shutdown(ctx) != nil {
			_ = s.http.Close()
		}
		<-s.served
		_ = s.listener.Close()
	})
}

// admit takes a handler in unless the server or the run is closing.
func (s *Server) admit() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	return s.workers == nil || s.workers.Admit()
}

func (s *Server) dismiss() {
	if s.workers != nil {
		s.workers.Dismiss()
	}
}

func peerOf(conn net.Conn) peer {
	unix, ok := conn.(*net.UnixConn)
	if !ok {
		return peer{err: errors.New("the connection is not a unix socket's")}
	}
	raw, err := unix.SyscallConn()
	if err != nil {
		return peer{err: err}
	}
	var cred *syscall.Ucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return peer{err: err}
	}
	if credErr != nil {
		return peer{err: credErr}
	}
	return peer{pid: int(cred.Pid)}
}

// fromHarness says whether the request's peer is the harness's process,
// alive with the start it was named with.
func (s *Server) fromHarness(ctx context.Context) error {
	p, _ := ctx.Value(peerKey{}).(peer)
	if p.err != nil {
		return fmt.Errorf("the connection's peer could not be read: %w", p.err)
	}
	s.mu.Lock()
	harness := s.harness
	s.mu.Unlock()
	if p.pid == 0 || p.pid != harness.PID {
		return fmt.Errorf("process %d is not the harness's own process; nothing it asks is served", p.pid)
	}
	if start, err := proc.StartTime(p.pid); err != nil || start != harness.Start {
		return errors.New("the harness's process is gone or was replaced")
	}
	return nil
}

// awaitHarness holds a request until the host names the harness, within the
// configured wait: the mod's first request can race the host's word.
func (s *Server) awaitHarness(ctx context.Context) error {
	timer := time.NewTimer(s.cfg.Wait)
	defer timer.Stop()
	select {
	case <-s.known:
		return nil
	case <-timer.C:
		return errors.New("the harness's process is not known to this run yet; nothing is served")
	case <-s.stop:
		return errClosing
	case <-ctx.Done():
		return ctx.Err()
	}
}

// errClosing refuses what comes or waits while the run ends.
var errClosing = errors.New("the run is ending; nothing is served")

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	if !s.admit() {
		refuse(w, http.StatusServiceUnavailable, errClosing.Error())
		return
	}
	defer s.dismiss()
	s.handle(w, r)
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if err := s.awaitHarness(r.Context()); err != nil {
		refuse(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err := s.fromHarness(r.Context()); err != nil {
		refuse(w, http.StatusForbidden, err.Error())
		return
	}
	switch r.URL.Path {
	case PathHello:
	case PathTurnStart, PathTurnEnd, PathEvents, PathSpaceHold, PathSpaceScreen, PathSpaceKeys, PathSpaceResize, PathSessionClosing:
		refuse(w, http.StatusNotImplemented, r.URL.Path+" is not in this build")
		return
	default:
		refuse(w, http.StatusNotFound, "no endpoint "+r.URL.Path)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		refuse(w, http.StatusMethodNotAllowed, "every endpoint is a POST")
		return
	}
	var hello HelloRequest
	if status, err := decode(w, r, &hello); err != nil {
		refuse(w, status, err.Error())
		return
	}
	if hello.LoadID == "" || hello.SessionID == "" || hello.HarnessVersion == "" {
		refuse(w, http.StatusBadRequest, "a hello names its load, its session and the harness's version")
		return
	}
	if err := s.bind(r.Context(), hello); errors.Is(err, errClosing) {
		refuse(w, http.StatusServiceUnavailable, err.Error())
		return
	} else if err != nil {
		// The harness was replaced while the body was read: the hello
		// proves nothing, and nobody is there to take a refusal.
		panic(http.ErrAbortHandler)
	}
	answer := HelloAnswer{Role: s.cfg.Role, HostsSpaces: s.cfg.HostsSpaces}
	if s.answering != nil {
		s.answering()
	}
	// The peer is checked again before the answer: a harness replaced since
	// the hello was bound is not the one that asked, and nobody is there to
	// take the answer, so the connection is cut with none.
	if s.fromHarness(r.Context()) != nil {
		panic(http.ErrAbortHandler)
	}
	reply(w, http.StatusOK, answer)
}

// bind makes hello the run's load, writes its record and tells the run, once
// the request's peer is checked again and the server is still open, all under
// one lock: a harness replaced while the body was read gets no effect of its
// hello — no load, no record, no capability made live — and a handler that
// outlasted Close's wait writes nothing it had not begun.
func (s *Server) bind(ctx context.Context, hello HelloRequest) error {
	s.bindMu.Lock()
	defer s.bindMu.Unlock()
	if err := s.fromHarness(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errClosing
	}
	s.load = &hello
	s.mu.Unlock()
	s.record(hello)
	if s.cfg.Admitted != nil {
		s.cfg.Admitted(hello)
	}
	return nil
}

// record writes the admitted hello where the run keeps it, whole: a
// temporary file moved over the last one.
func (s *Server) record(hello HelloRequest) {
	if s.cfg.Record == "" {
		return
	}
	data, err := json.Marshal(hello)
	if err != nil {
		return
	}
	temporary := s.cfg.Record + ".new"
	if state.WriteFile(temporary, append(data, '\n'), 0o600) != nil {
		_ = state.Discard(temporary)
		return
	}
	if state.Move(temporary, s.cfg.Record) != nil {
		_ = state.Discard(temporary)
	}
}

// decode reads a body within the protocol's bound into v, refusing a field
// the schema does not name and anything after the one value.
func decode(w http.ResponseWriter, r *http.Request, v any) (int, error) {
	body := http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return http.StatusRequestEntityTooLarge, fmt.Errorf("a body is at most %d bytes", MaxBodyBytes)
		}
		return http.StatusBadRequest, fmt.Errorf("the body is not the request this endpoint takes: %w", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return http.StatusRequestEntityTooLarge, fmt.Errorf("a body is at most %d bytes", MaxBodyBytes)
		}
		return http.StatusBadRequest, errors.New("the body carries more than one value")
	}
	return 0, nil
}

func refuse(w http.ResponseWriter, status int, why string) {
	reply(w, status, Refusal{Error: why})
}

func reply(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		status, data = http.StatusInternalServerError, []byte(`{"error":"the answer could not be encoded"}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(append(data, '\n'))
}
