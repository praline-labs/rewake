package pi

import (
	"context"
	"encoding/json"
	"regexp"
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

// carry asks the extension to carry a request out and words its reply. An
// interrupt names the turn this image matched to its start, attributed to
// the asker before it goes out, and waits for that turn's end (turns.go) up
// to control.SettleWait.
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
	sent := time.Now()
	ask := frame{Op: "control", Thread: thread, Action: request.Action, Request: request.ID, From: request.From, Focus: request.Focus}
	if request.Action == control.Interrupt {
		l.mu.Lock()
		ask.Turn = l.turn
		l.mu.Unlock()
		if ask.Turn == "" {
			return control.Answer{Outcome: control.Refused, Reason: control.NoTurn}
		}
		r.interrupts.Record(request, ask.Turn, sent)
	}
	asking, cancel := context.WithTimeout(ctx, 5*time.Second)
	reply, err := l.ask(asking, ask)
	cancel()
	if err != nil {
		return control.Answer{Outcome: control.Failed, Reason: control.CutShort, Detail: "the extension did not acknowledge the control request; its outcome is unknown", Open: true}
	}
	answer := worded(request.Action, reply)
	if request.Action == control.Interrupt {
		if answer.Outcome != control.Requested {
			r.interrupts.Forget(request.ID)
			return answer
		}
		return r.interrupts.Wait(ctx, request.ID, sent.Add(control.SettleWait), r.ctx.Done())
	}
	return r.controlAnswer(request, answer)
}

// worded is what a reply of the extension says of action: a refusal by the
// code of the check that made it, the native result read here.
func worded(action string, reply frame) control.Answer {
	if reply.Code != "" || reply.Outcome == control.Refused {
		return control.Refusal(action, reply.Code)
	}
	switch reply.Outcome {
	case control.Started, control.Requested:
		return control.Answer{Outcome: reply.Outcome}
	case control.Failed:
		if action != control.Compact {
			return control.Answer{Outcome: control.Failed, Detail: "native interrupt could not be requested"}
		}
		var failed struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(reply.Native, &failed)
		if nothingToCompact.MatchString(failed.Error) {
			return control.Answer{Outcome: control.Refused, Reason: control.NothingToCompact}
		}
		return control.Answer{Outcome: control.Failed, Reason: control.CutShort, Detail: "native compaction failed or was interrupted"}
	case control.Done:
		var compacted struct {
			TokensBefore *float64 `json:"tokensBefore"`
			TokensAfter  *float64 `json:"estimatedTokensAfter"`
		}
		_ = json.Unmarshal(reply.Native, &compacted)
		return control.Answer{Outcome: control.Done, TokensBefore: tokenCount(compacted.TokensBefore), TokensAfter: tokenCount(compacted.TokensAfter)}
	}
	return control.Answer{Outcome: control.Failed, Detail: "the extension returned an unknown control outcome", Open: true}
}

// nothingToCompact is how pi's compaction says the conversation is too short
// for one, read on its error's text.
var nothingToCompact = regexp.MustCompile(`(?i)already compacted|nothing to compact|session too small`)

func tokenCount(value *float64) *int64 {
	if value == nil || *value < 0 {
		return nil
	}
	count := int64(*value)
	return &count
}

// traced has the extension put an interrupted turn's actor into pi's
// history once its end was published, and settles the interrupt by whether
// it could.
func (r *run) traced(l *link, thread, actor string, settled func(bool)) {
	ctx, cancel := context.WithTimeout(r.ctx, 5*time.Second)
	defer cancel()
	answer, err := l.ask(ctx, frame{Op: "trace", Thread: thread, Text: actor})
	settled(err == nil && answer.OK)
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
		r.outcomeLocked(request.Request, request.From, worded(control.Compact, request))
	}
	return frame{OK: true}
}
