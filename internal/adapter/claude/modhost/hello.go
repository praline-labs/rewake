package modhost

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// hello binds a load of the mod to the run and answers what the run is.
func (s *Server) hello(w http.ResponseWriter, r *http.Request) {
	var hello HelloRequest
	if status, err := decode(w, r, &hello, MaxBodyBytes); err != nil {
		refuse(w, status, err.Error())
		return
	}
	if hello.LoadID == "" || hello.SessionID == "" || hello.HarnessVersion == "" {
		refuse(w, http.StatusBadRequest, "a hello names its load, its session and the harness's version")
		return
	}
	generation, err := s.bind(r.Context(), hello)
	var superseded errSuperseded
	switch {
	case errors.Is(err, errClosing):
		refuse(w, http.StatusServiceUnavailable, err.Error())
		return
	case errors.As(err, &superseded):
		holder := superseded.holder
		reply(w, http.StatusGone, Refusal{Error: err.Error(), LoadSeq: &holder})
		return
	case err != nil:
		// The harness was replaced while the body was read: the hello
		// proves nothing, and nobody is there to take a refusal.
		panic(http.ErrAbortHandler)
	}
	s.answer(w, r, HelloAnswer{Role: s.cfg.Role, Generation: generation})
}

// bind orders hello among the run's loads, writes its record and tells the
// run, once the request's peer is checked again and the server is still
// open, all under one lock: a harness replaced while the body was read gets
// no effect of its hello — no load, no record, no capability made live — and
// a handler that outlasted Close's wait writes nothing it had not begun. It
// answers the load's generation (alive.go, admitLoad).
func (s *Server) bind(ctx context.Context, hello HelloRequest) (int, error) {
	s.bindMu.Lock()
	defer s.bindMu.Unlock()
	if err := s.fromHarness(ctx); err != nil {
		return 0, err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return 0, errClosing
	}
	generation, transition, err := s.admitLoad(hello)
	s.mu.Unlock()
	if err != nil {
		return 0, err
	}
	if transition != 0 {
		s.record(hello)
		s.report(transition, true)
	}
	return generation, nil
}

// admittedLoad says the run admitted a hello of the load.
func (s *Server) admittedLoad(load string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loads[load]
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
