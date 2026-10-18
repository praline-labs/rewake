package modhost

import (
	"context"
	"net/http"
	"time"

	"github.com/praline-labs/rewake/internal/core/control"
	"github.com/praline-labs/rewake/internal/core/session"
)

type command struct {
	request    control.Request
	load       string
	generation int
	sent       bool
	answer     chan control.Answer
}

type management struct {
	cancel   context.CancelFunc
	wake     chan struct{}
	queued   *command
	requests map[string]*command
	order    []string
	observed observations
}

// StartControl keeps the file protocol with the run, counted until it stops.
func (s *Server) StartControl(parent context.Context) {
	if s.cfg.ControlDir == "" || !s.admit() {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	s.mu.Lock()
	s.management.cancel = cancel
	if s.closed {
		cancel()
	}
	s.mu.Unlock()
	go func() {
		defer s.dismiss()
		control.Serve(ctx, s.cfg.ControlDir, 50*time.Millisecond, s.carry, s.recordOutcome)
	}()
}

func (s *Server) carry(ctx context.Context, request control.Request) control.Answer {
	answer := s.dispatch(ctx, request)
	if answer.Outcome != control.Requested && answer.Outcome != control.Started && !answer.Open {
		s.recordOutcome(request, answer)
	}
	return answer
}

func (s *Server) dispatch(ctx context.Context, request control.Request) control.Answer {
	s.mu.Lock()
	current := s.current
	if s.closed || current == nil || !current.live {
		s.mu.Unlock()
		return control.Answer{Outcome: control.Refused, Reason: control.NotAnswering}
	}
	if request.Action != control.Compact && request.Action != control.Interrupt {
		s.mu.Unlock()
		return control.Answer{Outcome: control.Refused, Detail: "unknown control action"}
	}
	thread := s.management.observed.snapshot.Thread
	if thread == "" {
		thread = current.hello.SessionID
	}
	if request.Conversation != "" && request.Conversation != thread {
		s.mu.Unlock()
		return control.Answer{Outcome: control.Refused, Reason: control.NotSelected}
	}
	request.Conversation = thread
	pending := &command{request: request, load: current.hello.LoadID, generation: current.generation, answer: make(chan control.Answer, 1)}
	m := &s.management
	m.requests[request.ID] = pending
	m.order = append(m.order, request.ID)
	if len(m.order) > 64 {
		delete(m.requests, m.order[0])
		m.order = m.order[1:]
	}
	m.queued = pending
	select {
	case m.wake <- struct{}{}:
	default:
	}
	s.mu.Unlock()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case answer := <-pending.answer:
		return answer
	case <-timer.C:
	case <-s.stop:
	case <-ctx.Done():
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if m.queued == pending {
		m.queued = nil
	}
	if !pending.sent {
		delete(m.requests, request.ID)
		return control.Answer{Outcome: control.Refused, Reason: control.NotAnswering}
	}
	return control.Answer{Outcome: control.Failed, Reason: control.CutShort, Detail: "the mod did not acknowledge the request; its outcome is unknown", Open: true}
}

// takeCommand checks the generation again after the held poll wakes.
func (s *Server) takeCommand(poll AliveRequest) *ControlCommand {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending := s.management.queued
	if s.closed || s.current == nil || !s.current.live || s.current.hello.LoadID != poll.LoadID || s.current.generation != poll.Generation || pending == nil || pending.load != poll.LoadID || pending.generation != poll.Generation {
		return nil
	}
	pending.sent = true
	s.management.queued = nil
	request := pending.request
	return &ControlCommand{ID: request.ID, Action: request.Action, From: request.From, Focus: request.Focus, Conversation: request.Conversation}
}

func (s *Server) controlReply(w http.ResponseWriter, r *http.Request) {
	var result ControlReply
	if status, err := decode(w, r, &result, MaxBodyBytes); err != nil {
		refuse(w, status, err.Error())
		return
	}
	switch result.Outcome {
	case control.Requested, control.Done, control.Refused, control.Failed:
	default:
		refuse(w, http.StatusBadRequest, "unknown control outcome")
		return
	}
	if err := s.fromHarness(r.Context()); err != nil {
		s.refuseEffect(w, http.StatusForbidden, err)
		return
	}
	s.mu.Lock()
	if status, err := s.currentGeneration(result.LoadID, result.Generation); err != nil {
		s.mu.Unlock()
		s.refuseEffect(w, status, err)
		return
	}
	pending := s.management.requests[result.ID]
	if pending == nil || !pending.sent || pending.load != result.LoadID || pending.generation != result.Generation {
		s.mu.Unlock()
		refuse(w, http.StatusConflict, "no command of this generation has that id")
		return
	}
	answer := control.Answer{Outcome: result.Outcome, Reason: result.Reason, Detail: result.Detail, Open: result.Open, TokensBefore: tokenCount(result.TokensBefore), TokensAfter: tokenCount(result.TokensAfter)}
	if result.Outcome != control.Requested && !result.Open {
		s.outcomeLocked(pending.request, answer)
	}
	select {
	case pending.answer <- answer:
	default:
	}
	s.mu.Unlock()
	s.answer(w, r, Done{})
}

func (s *Server) recordOutcome(request control.Request, answer control.Answer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outcomeLocked(request, answer)
}

func (s *Server) outcomeLocked(request control.Request, answer control.Answer) {
	if request.Action != control.Compact {
		return
	}
	observed := &s.management.observed
	for _, known := range observed.outcomes {
		if known.Request == request.ID {
			return
		}
	}
	observed.outcomes = session.KeepOutcome(observed.outcomes, session.CompactionOutcome{
		Request: request.ID, RequestedBy: request.From, Outcome: answer.Outcome, Reason: answer.Reason, Detail: answer.Detail,
		TokensBefore: answer.TokensBefore, TokensAfter: answer.TokensAfter, EndedAt: time.Now(),
	})
}
