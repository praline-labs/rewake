package pi

import (
	"context"
	_ "embed"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/infra/state"
)

//go:embed extension.ts
var extension []byte

type run struct {
	file, socket string
	ctx          context.Context
	cancel       context.CancelFunc
	listener     *state.Listener
	handler      adapter.TurnHandler
	workers      adapter.Workers
	live         func([]adapter.Capability)
	liveMu       sync.Mutex
	mu           sync.Mutex
	closing      bool
	links        map[*link]struct{}
	current      *link
	thread       string
	process      adapter.RunProcess
	ready        chan struct{}
	processOnce  sync.Once
	jobs         sync.WaitGroup
}

func newRun(file, socket string) *run {
	return &run{file: file, socket: socket, links: make(map[*link]struct{}), ready: make(chan struct{})}
}

func (r *run) ReportTurns(handler adapter.TurnHandler)      { r.handler = handler }
func (r *run) ReportLive(report func([]adapter.Capability)) { r.live = report }
func (r *run) CountWorkers(workers adapter.Workers)         { r.workers = workers }
func (r *run) HarnessStarted(process adapter.RunProcess) {
	r.processOnce.Do(func() { r.process = process; close(r.ready) })
}

func (r *run) Start(ctx context.Context, _ func(string)) error {
	r.ctx, r.cancel = context.WithCancel(ctx)
	if err := state.WriteFile(r.file, extension, 0o600); err != nil {
		return err
	}
	listener, err := state.Listen(r.socket)
	if err != nil {
		return err
	}
	r.listener = listener
	go r.accept()
	return nil
}

func (r *run) accept() {
	for {
		conn, err := r.listener.Accept()
		if err != nil {
			return
		}
		l := newLink(conn)
		r.mu.Lock()
		if r.closing {
			r.mu.Unlock()
			l.close()
			return
		}
		r.links[l] = struct{}{}
		r.mu.Unlock()
		go r.serve(l)
	}
}

// Callbacks can write the room. Count them before admitting them, so a slow
// publication outlives Close without losing the room lease underneath it.
func (r *run) enter() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closing || (r.workers != nil && !r.workers.Admit()) {
		return false
	}
	r.jobs.Add(1)
	return true
}

func (r *run) leave() {
	if r.workers != nil {
		r.workers.Dismiss()
	}
	r.jobs.Done()
}

func (r *run) Close() {
	r.mu.Lock()
	r.closing = true
	r.current, r.thread = nil, ""
	if r.cancel != nil {
		r.cancel()
	}
	for l := range r.links {
		l.close()
	}
	r.mu.Unlock()
	if r.listener != nil {
		_ = r.listener.Close()
	}
	done := make(chan struct{})
	go func() { r.jobs.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
	}
}

// The live callback must be serialized without holding the lock taken by
// Deliver and Thread. A disconnected older socket cannot withdraw its successor.
func (r *run) change(l *link, thread string) bool {
	r.liveMu.Lock()
	defer r.liveMu.Unlock()
	r.mu.Lock()
	if l != nil && (r.closing || l.gone()) {
		r.mu.Unlock()
		return false
	}
	old := r.current
	r.current, r.thread = l, thread
	r.mu.Unlock()
	if old != nil && old != l {
		old.close()
	}
	if r.live != nil && r.enter() {
		if l == nil {
			r.live(nil)
		} else {
			r.live([]adapter.Capability{adapter.WakeCapability, adapter.TurnBoundaryCapability})
		}
		r.leave()
	}
	return true
}

func (r *run) disconnect(l *link) {
	l.close()
	r.liveMu.Lock()
	defer r.liveMu.Unlock()
	r.mu.Lock()
	delete(r.links, l)
	current := r.current == l
	if current {
		r.current = nil
		r.thread = ""
	}
	r.mu.Unlock()
	if current && r.live != nil && r.enter() {
		r.live(nil)
		r.leave()
	}
}

func (r *run) connection() (*link, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closing || r.current == nil || r.current.gone() {
		return nil, ""
	}
	return r.current, r.thread
}

func (r *run) on(l *link) bool { current, _ := r.connection(); return current == l && l != nil }

func (r *run) peer(conn net.Conn) error {
	select {
	case <-r.ready:
	case <-r.ctx.Done():
		return r.ctx.Err()
	case <-time.After(5 * time.Second):
		return errors.New("the harness process was not announced")
	}
	return sameProcess(conn, r.process)
}
