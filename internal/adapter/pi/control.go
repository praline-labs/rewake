package pi

import (
	"context"
	"time"

	"github.com/praline-labs/rewake/internal/core/control"
	"github.com/praline-labs/rewake/internal/core/session"
)

func (r *run) serveControl() {
	if r.controlDir == "" || !r.enter() {
		return
	}
	go func() {
		defer r.leave()
		control.Serve(r.ctx, r.controlDir, 50*time.Millisecond, r.carry, r.recordOutcome)
	}()
}

func (r *run) carry(ctx context.Context, request control.Request) control.Answer {
	l, thread := r.connection()
	if l == nil {
		return r.controlAnswer(request, control.Answer{Outcome: control.Refused, Reason: control.NotAnswering})
	}
	if request.Action != control.Compact && request.Action != control.Interrupt {
		return r.controlAnswer(request, control.Answer{Outcome: control.Refused, Detail: "unknown control action"})
	}
	if request.Conversation != "" && request.Conversation != thread {
		return r.controlAnswer(request, control.Answer{Outcome: control.Refused, Reason: control.NotSelected})
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	reply, err := l.ask(ctx, frame{Op: "control", Thread: thread, Action: request.Action, Request: request.ID, From: request.From, Focus: request.Focus})
	if err != nil {
		return control.Answer{Outcome: control.Failed, Reason: control.CutShort, Detail: "the extension did not acknowledge the control request; its outcome is unknown", Open: true}
	}
	answer := control.Answer{Outcome: reply.Outcome, Reason: reply.Reason, Detail: reply.Detail, Open: reply.Open, TokensBefore: reply.TokensBefore, TokensAfter: reply.TokensAfter}
	switch answer.Outcome {
	case control.Done, control.Refused, control.Failed, control.Started, control.Requested:
	default:
		return control.Answer{Outcome: control.Failed, Detail: "the extension returned an unknown control outcome", Open: true}
	}
	return r.controlAnswer(request, answer)
}

func (r *run) controlAnswer(request control.Request, answer control.Answer) control.Answer {
	if answer.Outcome != control.Started && answer.Outcome != control.Requested && !answer.Open {
		r.recordOutcome(request, answer)
	}
	return answer
}

func (r *run) recordOutcome(request control.Request, answer control.Answer) {
	if request.Action != control.Compact {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.outcomeLocked(request.ID, request.From, answer)
}

func (r *run) outcomeLocked(request, from string, answer control.Answer) {
	// The native completion can arrive before the initial control answer;
	// the observer keeps one outcome per request, a final immediate refusal
	// among them.
	r.observed.view.Outcome(session.CompactionOutcome{
		Request: request, RequestedBy: from, Outcome: answer.Outcome, Reason: answer.Reason, Detail: answer.Detail,
		TokensBefore: answer.TokensBefore, TokensAfter: answer.TokensAfter, EndedAt: time.Now(),
	})
}

func (r *run) compacted(l *link, request frame) frame {
	r.mu.Lock()
	defer r.mu.Unlock()
	if l != r.current || request.Thread != r.thread {
		return frame{Error: "compaction belongs to another conversation"}
	}
	if request.Op == "compacted" {
		if request.Sequence == 0 {
			return frame{Error: "a compaction needs its event sequence"}
		}
		r.observed.view.Compaction(l, request.Sequence, request.From, request.Request)
	} else {
		switch request.Outcome {
		case control.Done, control.Refused, control.Failed:
		default:
			return frame{Error: "unknown compaction outcome"}
		}
		r.outcomeLocked(request.Request, request.From, control.Answer{Outcome: request.Outcome, Reason: request.Reason, Detail: request.Detail, TokensBefore: request.TokensBefore, TokensAfter: request.TokensAfter})
	}
	return frame{OK: true}
}
