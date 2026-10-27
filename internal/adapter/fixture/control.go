//go:build rewaketest

package fixture

import (
	"context"
	"time"

	"github.com/praline-labs/rewake/internal/core/control"
	"github.com/praline-labs/rewake/internal/core/session"
)

// The served side of rewake compact and rewake interrupt
// (docs/remote-control.md): the run takes each request from its control
// directory (control.Serve) and carries it to the program over the
// connection while Control is live there, so a request reaches only what
// proved it can take one.

// controlPoll is how often the run looks at its control directory.
const controlPoll = 50 * time.Millisecond

// serveControl serves the run's control directory until the backend closes.
func (b *backend) serveControl() {
	if b.controlDir == "" {
		return
	}
	b.mu.Lock()
	if b.closing {
		b.mu.Unlock()
		return
	}
	b.routines.Add(1)
	b.mu.Unlock()
	go func() {
		defer b.routines.Done()
		control.Serve(b.ctx, b.controlDir, controlPoll, b.carry, b.withdrawnRequest)
	}()
}

// carry hands one request to the program and answers what it answered.
// Without a live Control nothing is sent: the request is not answered by
// anything that could carry it out. A request sent and then lost — the
// connection gone, no answer within the bound — may still be carried out,
// so its failure leaves the outcome open.
func (b *backend) carry(_ context.Context, request control.Request) control.Answer {
	l, live := b.isLive(Control)
	if !live {
		return control.Answer{Outcome: control.Refused, Reason: control.NotAnswering}
	}
	answer, err := l.ask(Frame{Op: opControl, Request: request.ID, Action: request.Action, Focus: request.Focus, From: request.From}, b.bound)
	if err != nil {
		return control.Answer{Outcome: control.Failed, Reason: control.CutShort, Detail: err.Error(), Open: true}
	}
	return control.Answer{Outcome: answer.Outcome, Reason: answer.Reason, Detail: answer.Detail}
}

// withdrawnRequest records the outcome of a compaction withdrawn before it was
// taken, as the program's own outcome is recorded: the asking main's letter
// closes with it.
func (b *backend) withdrawnRequest(request control.Request, answer control.Answer) {
	if request.Action != control.Compact {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.observed.Outcome(session.CompactionOutcome{
		Request: request.ID, RequestedBy: request.From, Outcome: answer.Outcome, Reason: answer.Reason, EndedAt: time.Now(),
	})
}
