package codex

import (
	"context"
	"errors"
	"time"

	"github.com/praline-labs/rewake/internal/control"
	"github.com/praline-labs/rewake/internal/harness/codex/gateway"
)

// controlPoll is how often the wrapper looks for a main's request; the asker
// gives the served side five seconds to take one (docs/remote-control.md).
const controlPoll = 100 * time.Millisecond

// interruptWait bounds an interrupt, answered within the asker's own wait of
// 10 seconds. A compaction is answered once it has started, within
// compactStart, and its end is waited for apart from the asker for as long as
// its running mark holds (gateway.CompactionWait); that end reaches main as a
// letter.
const interruptWait = 8 * time.Second

// compactStart bounds the wait for a compaction's start, well inside the ten
// seconds the asker waits for an answer.
const compactStart = 3 * time.Second

// steer carries out a main's request on the owned app-server: Codex has no
// plugin, and the wrapper is what holds the connection and knows the turn.
func (s *serverSession) steer(ctx context.Context, request control.Request) control.Answer {
	if request.Action != control.Compact && request.Action != control.Interrupt && request.Action != control.Accept {
		return control.Answer{Outcome: control.Failed, Detail: "unknown action " + request.Action}
	}
	if s.gateway == nil {
		// Not reached while the control directory is served: Serve starts
		// only once the gateway is there. Nothing could record this answer as
		// the outcome either, as the telemetry lives in the gateway.
		return control.Answer{Outcome: control.Failed, Detail: "the app-server is not connected yet"}
	}
	if request.Action == control.Accept {
		return s.accept(request.Conversation)
	}
	if request.Action == control.Interrupt {
		ctx, cancel := context.WithTimeout(ctx, interruptWait)
		defer cancel()
		return s.gateway.Interrupt(ctx, request.From)
	}
	ctx, cancel := context.WithTimeout(ctx, s.gateway.CompactionWait())
	if request.Focus != "" {
		// The command refuses a focus for Codex before sending; one that
		// arrives anyway is not dropped silently, and is its outcome too, as
		// every final answer the gateway gives is.
		cancel()
		answer := control.Answer{Outcome: control.Failed, Detail: "Codex takes no focus for a compaction"}
		s.gateway.CompactionEnded(request.ID, request.From, answer)
		return answer
	}
	answer, later := s.gateway.Compact(ctx, request.ID, request.From, compactStart)
	if later == nil {
		cancel()
		return answer
	}
	go func() {
		defer cancel()
		s.gateway.CompactionEnded(request.ID, request.From, later())
	}()
	return answer
}

// withdrawn records a compaction withdrawn before it was taken as its outcome,
// as the gateway does every final answer of its own: a command whose wait was
// cut short holds an open answer, and main's letter comes from this at once
// rather than at its bound.
func (s *serverSession) withdrawn(request control.Request, answer control.Answer) {
	if request.Action == control.Compact && s.gateway != nil {
		s.gateway.CompactionEnded(request.ID, request.From, answer)
	}
}

// accept takes the selected conversation for the launch's own, at the
// person's word (rewake accept): deliveries held for it go on.
func (s *serverSession) accept(conversation string) control.Answer {
	err := s.gateway.Accept(conversation)
	switch {
	case err == nil:
		return control.Answer{Outcome: control.Done}
	case errors.Is(err, gateway.ErrNothingHeld):
		return control.Answer{Outcome: control.Refused, Reason: control.NothingHeld}
	case errors.Is(err, gateway.ErrNoSelection):
		return control.Answer{Outcome: control.Refused, Reason: control.NoSelection}
	case errors.Is(err, gateway.ErrNotSelected):
		detail := ""
		if b := s.gateway.Binding(); b.Ready {
			detail = "the selected conversation is " + b.Thread
		}
		return control.Answer{Outcome: control.Refused, Reason: control.NotSelected, Detail: detail}
	}
	return control.Answer{Outcome: control.Failed, Detail: err.Error()}
}
