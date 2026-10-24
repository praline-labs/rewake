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
	if hello.Op != "hello" || hello.Protocol != protocolVersion || hello.Thread == "" || hello.State == nil || hello.State.Sequence == 0 || (hello.State.Activity != "working" && hello.State.Activity != "idle") {
		l.answer(hello, frame{Error: "expected protocol 2 hello with a conversation and state"})
		return
	}
	if r.handler.Publish == nil || r.handler.Capture == nil {
		l.answer(hello, frame{Error: "the host takes no turn ends"})
		return
	}
	// Put hello's acknowledgment on the wire before publishing live Wake:
	// delivery can start as soon as the live callback records the capability.
	l.answer(hello, frame{OK: true})
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
		completion, err := r.endTurn(l, request)
		if err != nil {
			r.leave()
			l.answer(request, frame{Error: err.Error()})
			return
		}
		// Capture belongs to the reader; publication must not block answers to a
		// wake the host sends while it handles this end.
		go func() { defer r.leave(); l.answer(request, r.publish(l, completion)) }()
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

func (r *run) startTurn(l *link, request frame) frame {
	if request.Turn == "" || l.turn != "" {
		return frame{Error: "a start needs a new turn"}
	}
	_, thread := r.connection()
	if request.Thread != thread {
		return frame{Error: "the turn belongs to another conversation"}
	}
	l.turn, l.started = request.Turn, boottime.Now()
	if r.handler.Started != nil {
		// The extension has no boot-clock timestamp. Do not turn the arrival
		// time into evidence about a previous turn whose end was lost.
		r.handler.Started(thread, request.Turn, 0)
	}
	return frame{OK: true}
}

func (r *run) endTurn(l *link, request frame) (adapter.TurnEnd, error) {
	_, thread := r.connection()
	if request.Turn == "" || request.Turn != l.turn || request.Thread != thread {
		return adapter.TurnEnd{}, errors.New("the end has no matching start in this conversation")
	}
	reason := adapter.EndReason(request.Outcome)
	if reason != adapter.Answered && reason != adapter.Aborted && reason != adapter.Failed {
		return adapter.TurnEnd{}, errors.New("unknown turn outcome")
	}
	completion := adapter.TurnEnd{Event: ID + "/" + request.Turn, Thread: thread, Reason: reason, Answer: request.Text, Started: l.started}
	if r.handler.Capture != nil {
		completion.Boundary = r.handler.Capture()
	}
	completion.Ended = boottime.Now()
	if err := completion.Check(); err != nil {
		return adapter.TurnEnd{}, err
	}
	l.turn, l.started = "", 0
	return completion, nil
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
