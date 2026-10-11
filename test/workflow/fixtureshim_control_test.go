package workflow

// The fixture program's side of rewake compact and rewake interrupt: the
// adapter serves the run's control directory and carries each request to the
// program while Control is live on the connection; the program carries it out
// in its own session and answers. A compaction is answered started at once and
// ends compactTakes later, reported to the adapter with its counts; an
// interrupt ends the held turn with the interrupter's name as its text.
// RW_SHIM_NO_CONTROL leaves Control out of the hello, so nothing is carried.

import (
	"os"
	"time"
)

// shimCompactTakes, a duration, is how long a compaction takes after its
// start, so a case can see what happens between its start and its end.
const shimCompactTakes = "RW_SHIM_COMPACT_TAKES"

// shimHoldForInterrupt holds the session's first turn open until it is
// interrupted or interruptWindow passes: notes steered into it do not end it.
const shimHoldForInterrupt = "RW_SHIM_HOLD_FOR_INTERRUPT"

// shimNewThread names the conversation the session starts before it works the
// first notice that reaches it, as a person's /clear does, and reports to the
// adapter.
const shimNewThread = "RW_SHIM_NEW_THREAD"

// interruptWindow bounds a turn held for an interrupt: long enough for a case
// to compact and interrupt it, short enough to leave the session's ceiling
// room to end.
const interruptWindow = 12 * time.Second

// The tokens a fixture compaction reports around itself.
const (
	fixtureTokensBefore = 120000
	fixtureTokensAfter  = 9000
)

// controlRequest and controlAnswer are a request as the adapter carries it
// and the program's answer.
type controlRequest struct {
	ID     string
	Action string
	Focus  string
	From   string
}

type controlAnswer struct {
	Outcome string
	Reason  string
}

// control answers one request the adapter carried from the run's control
// directory.
func (s *fixtureSession) control(frame, reply fixtureFrame) fixtureFrame {
	answer := s.act(controlRequest{ID: frame.Request, Action: frame.Action, Focus: frame.Focus, From: frame.From})
	reply.OK, reply.Outcome, reply.Reason = true, answer.Outcome, answer.Reason
	return reply
}

// act carries one request out, with the refusals the harness gives.
func (s *fixtureSession) act(asked controlRequest) controlAnswer {
	s.turn.mu.Lock()
	defer s.turn.mu.Unlock()
	switch asked.Action {
	case "compact":
		if s.turn.open != "" {
			return controlAnswer{Outcome: "refused", Reason: "in a turn"}
		}
		if s.turn.compacting {
			return controlAnswer{Outcome: "refused", Reason: "another request in flight"}
		}
		s.turn.compacting = true
		s.recordTurnEvent("compact", asked.ID, asked.Focus)
		go s.compact(asked)
		return controlAnswer{Outcome: "started"}
	case "interrupt":
		if s.turn.open == "" || s.turn.abort == nil {
			return controlAnswer{Outcome: "refused", Reason: "no turn running"}
		}
		s.turn.aborted, s.turn.interrupter = true, asked.From
		select {
		case <-s.turn.abort:
		default:
			close(s.turn.abort)
		}
		return controlAnswer{Outcome: "done"}
	}
	return controlAnswer{Outcome: "failed", Reason: "the fixture does not take " + asked.Action}
}

// compact runs one compaction to its end and reports it with its counts.
func (s *fixtureSession) compact(asked controlRequest) {
	takes, err := time.ParseDuration(os.Getenv(shimCompactTakes))
	if err != nil {
		takes = 0
	}
	time.Sleep(takes)
	before, after := int64(fixtureTokensBefore), int64(fixtureTokensAfter)
	_ = s.send(fixtureFrame{
		Op: "compacted", Request: asked.ID, From: asked.From, Outcome: "done",
		TokensBefore: &before, TokensAfter: &after,
	})
	s.turn.mu.Lock()
	s.turn.compacting = false
	s.turn.mu.Unlock()
}

// changeThread starts the conversation shimNewThread names, once, and tells
// the adapter, before the turn of the notice that reached the session.
func (s *fixtureSession) changeThread() {
	next := os.Getenv(shimNewThread)
	if next == "" {
		return
	}
	s.mu.Lock()
	if s.thread == next {
		s.mu.Unlock()
		return
	}
	s.thread = next
	s.mu.Unlock()
	_ = s.send(fixtureFrame{Op: "thread", Thread: next})
	s.recordTurnEvent("thread", "", next)
}

// takeInterrupter is who interrupted the turn that just ended, cleared for the
// next one.
func (s *fixtureSession) takeInterrupter() string {
	s.turn.mu.Lock()
	defer s.turn.mu.Unlock()
	by := s.turn.interrupter
	s.turn.interrupter = ""
	return by
}

// currentThread is the conversation the session is in now.
func (s *fixtureSession) currentThread() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.thread
}
