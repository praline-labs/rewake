package pi

import (
	"bufio"
	"context"
	"errors"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/infra/boottime"
)

func (r *run) serve(l *link) {
	defer r.disconnect(l)
	if err := r.peer(l.conn); err != nil {
		return
	}
	_ = l.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	reader := bufio.NewReader(l.conn)
	hello, err := readFrame(reader)
	if err != nil {
		return
	}
	// An extension of another protocol is refused with the host's reason: an
	// old one warns and tries again, harmlessly, until the room's restart
	// writes the run's extension anew.
	if hello.Op == "hello" && hello.Protocol != protocolVersion {
		l.answer(hello, frame{Error: olderProtocol})
		return
	}
	if hello.Op != "hello" || hello.Thread == "" || hello.State == nil || hello.State.Sequence == 0 {
		l.answer(hello, frame{Error: "expected a hello with a conversation and state"})
		return
	}
	if r.handler.Publish == nil || r.handler.Capture == nil {
		l.answer(hello, frame{Error: "the host takes no turn ends"})
		return
	}
	// Put hello's acknowledgment on the wire before publishing live Wake:
	// delivery can start as soon as the live callback records the capability.
	l.answer(hello, frame{OK: true, Mirror: r.mirror != nil, Tools: r.table()})
	if !r.change(l, hello) {
		return
	}
	_ = l.conn.SetReadDeadline(time.Time{})
	go r.pollState(l, hello.Thread)
	l.serve(reader, func(request frame) { r.handle(l, request) })
}

func (r *run) handle(l *link, request frame) {
	if !r.on(l) || !r.enter() {
		l.answer(request, frame{Error: "the extension connection is no longer current"})
		return
	}
	switch request.Op {
	case "state":
		ok := r.observe(l, request.Thread, request.State)
		r.leave()
		l.answer(request, frame{OK: ok})
	case "compacted", "compaction-result":
		reply := r.compacted(l, request)
		r.leave()
		l.answer(request, reply)
	case "turn-start":
		reply := r.startTurn(l, request)
		r.leave()
		l.answer(request, reply)
	case "turn-end":
		completion, settled, err := r.endTurn(l, request)
		if err != nil {
			r.leave()
			l.answer(request, frame{Error: err.Error()})
			return
		}
		// Capture belongs to the reader; publication must not block answers to a
		// wake the host sends while it handles this end, nor the trace of an
		// interrupt, which is asked of the extension once the end is answered.
		r.publishing.Add(1)
		go func() {
			defer r.leave()
			defer r.publishing.Add(-1)
			l.answer(request, r.publish(l, completion))
			if settled != nil {
				r.traced(l, completion.Thread, completion.Answer, settled)
			}
		}()
	case "events":
		reply := r.mirrored(request)
		r.leave()
		l.answer(request, reply)
	case "tool-plan":
		reply := r.toolPlan(request)
		r.leave()
		l.answer(request, reply)
	case "tool-result":
		reply := r.toolResult(request)
		r.leave()
		l.answer(request, reply)
	case "closing":
		// pi quits: the shell's closing screen and the workers' close start
		// now rather than at the process's end (docs/shell.md).
		if r.onClosing != nil {
			r.onClosing()
		}
		r.leave()
		l.answer(request, frame{OK: true})
	default:
		r.leave()
		l.answer(request, frame{Error: "unknown extension operation"})
	}
}

// olderProtocol refuses a hello of an extension of another protocol.
const olderProtocol = "this extension speaks an older protocol of rewake: restart the room"

func (r *run) startTurn(l *link, request frame) frame {
	if request.Turn == "" || l.turn != "" {
		return frame{Error: "a start needs a new turn"}
	}
	_, thread := r.connection()
	if request.Thread != thread {
		return frame{Error: "the turn belongs to another conversation"}
	}
	l.mu.Lock()
	l.turn, l.started = request.Turn, boottime.Now()
	l.mu.Unlock()
	if r.handler.Started != nil {
		// The extension has no boot-clock timestamp. Do not turn the arrival
		// time into evidence about a previous turn whose end was lost.
		r.handler.Started(thread, request.Turn, 0)
	}
	return frame{OK: true}
}

// endTurn captures the end of the turn the link started. Main's interrupt
// of it puts the actor in place of the turn's own answer, and answers
// settled, to be called once the extension traced the actor in pi's history.
func (r *run) endTurn(l *link, request frame) (adapter.TurnEnd, func(bool), error) {
	_, thread := r.connection()
	if request.Turn == "" || request.Turn != l.turn || request.Thread != thread {
		return adapter.TurnEnd{}, nil, errors.New("the end has no matching start in this conversation")
	}
	reason := adapter.EndReason(request.Outcome)
	if reason != adapter.Answered && reason != adapter.Aborted && reason != adapter.Failed {
		return adapter.TurnEnd{}, nil, errors.New("unknown turn outcome")
	}
	answer := request.Text
	actor, settled := r.interrupts.Ended(request.Turn, reason == adapter.Aborted, time.Now())
	if actor != "" {
		answer = actor
	}
	completion := adapter.TurnEnd{Event: ID + "/" + request.Turn, Thread: thread, Reason: reason, Answer: answer, Started: l.started}
	if r.handler.Capture != nil {
		completion.Boundary = r.handler.Capture()
	}
	completion.Ended = boottime.Now()
	if err := completion.Check(); err != nil {
		return adapter.TurnEnd{}, nil, err
	}
	l.mu.Lock()
	l.turn, l.started = "", 0
	l.mu.Unlock()
	return completion, settled, nil
}

func (r *run) publish(l *link, completion adapter.TurnEnd) frame {
	// Retry only this captured end, with a new deadline per attempt. There is
	// no transcript recovery or durable outbox in this adapter.
	for attempt := range 3 {
		if attempt > 0 {
			select {
			case <-r.ctx.Done():
				return frame{Error: "the run is closing"}
			case <-time.After(200 * time.Millisecond):
			}
		}
		if !r.on(l) {
			return frame{Error: "the end connection closed"}
		}
		ctx, cancel := context.WithTimeout(r.ctx, 5*time.Second)
		err := r.handler.Publish(ctx, completion)
		cancel()
		if err == nil || errors.Is(err, adapter.ErrRunGone) {
			return frame{OK: true}
		}
	}
	return frame{Error: "the host did not take the turn end after three attempts"}
}
