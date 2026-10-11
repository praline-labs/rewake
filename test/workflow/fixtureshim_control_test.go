package workflow

// The fixture program's served side of rewake compact and rewake interrupt:
// it polls the run's control directory, as Claude Code's module did before S9,
// and carries each request out in its own session. A compaction is answered
// started at once and ends compactTakes later, reported to the adapter with
// its counts; an interrupt ends the held turn with the interrupter's name as
// its text. The protocol is the files of docs/remote-control.md, spelled out
// here rather than imported, like the rest of the exchange.
//
// S11 moves the served side behind the fixture's live Control; until then
// the program serves the directory itself, and RW_SHIM_NO_CONTROL leaves it
// unserved.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
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

// controlPoll is how often the program looks at the directory.
const controlPoll = 50 * time.Millisecond

var controlID = regexp.MustCompile(`^[0-9a-f]{32}$`)

// controlRequest and controlAnswer are a request's and an answer's files.
type controlRequest struct {
	ID     string `json:"id"`
	Action string `json:"action"`
	Focus  string `json:"focus,omitempty"`
	From   string `json:"from"`
}

type controlAnswer struct {
	ID      string `json:"id"`
	Outcome string `json:"outcome"`
	Reason  string `json:"reason,omitempty"`
}

// serveControl takes one request at a time until the process ends: mark it
// taken, check it is still in place, act, answer once.
func (s *fixtureSession) serveControl(dir string) {
	last := ""
	for {
		time.Sleep(controlPoll)
		path := filepath.Join(dir, "request.json")
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var asked controlRequest
		if json.Unmarshal(raw, &asked) != nil || !controlID.MatchString(asked.ID) || asked.ID == last {
			continue
		}
		last = asked.ID
		taken := filepath.Join(dir, asked.ID+".taken")
		if _, err := os.Stat(taken); err == nil {
			continue
		}
		if os.WriteFile(taken, nil, 0o600) != nil {
			continue
		}
		if again, err := os.ReadFile(path); err != nil || !bytes.Equal(again, raw) {
			writeControlAnswer(dir, controlAnswer{ID: asked.ID, Outcome: "refused", Reason: "withdrawn before it was taken"})
			continue
		}
		answer := s.act(asked)
		answer.ID = asked.ID
		writeControlAnswer(dir, answer)
	}
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

// writeControlAnswer puts the answer in place whole.
func writeControlAnswer(dir string, answer controlAnswer) {
	encoded, err := json.Marshal(answer)
	if err != nil {
		return
	}
	temporary := filepath.Join(dir, "."+answer.ID+".result.tmp")
	if os.WriteFile(temporary, encoded, 0o600) != nil {
		return
	}
	if os.Rename(temporary, filepath.Join(dir, answer.ID+".result")) != nil {
		_ = os.Remove(temporary)
	}
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
