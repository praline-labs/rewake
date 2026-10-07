//go:build rewakefixture

package fixture

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/sessionstate"
)

// endAttempts bounds how often an end is handed to the core before the
// program is told it failed; endDeadline is each attempt's own deadline, a
// variable so the package's tests can shorten it, taken by a backend when made.
const endAttempts = 3

var (
	endDeadline = 10 * time.Second
	endBackoff  = 200 * time.Millisecond
)

// turnRecord is what the adapter noted at a turn's start: the moment, and the
// boundary captured then, which no later read is part of.
type turnRecord struct {
	started  int64
	boundary *inbox.ReadBoundary
}

// telemetry is the last activity the program reported.
type telemetry struct {
	activity string
	at       time.Time
}

// handle takes one frame on the reader. What fixes a frame's moment — the
// liveness of the connection it came on, the read boundary of a turn's start
// or end — is done here, before the next line is read, so no read the core
// acknowledges after the frame arrived is part of it; what waits for the core
// or for the program runs on its own goroutine.
func (b *backend) handle(l *link, frame Frame) {
	switch frame.Op {
	case opTurnStarted:
		reply := b.turnStarted(l, frame)
		b.dispatch(frame, func() { _ = l.answer(frame, reply) })
	case opTurnEnded:
		completion, err := b.endOf(l, frame)
		b.dispatch(frame, func() { _ = l.answer(frame, b.decide(l, completion, frame.Hold, err)) })
	case opActivity:
		b.activity(l, frame)
	default:
		if frame.ID != 0 {
			b.dispatch(frame, func() { _ = l.answer(frame, Frame{Error: "the adapter does not take " + frame.Op}) })
		}
	}
}

// turnStarted notes the turn's start on the boot clock and captures its read
// boundary, as Codex's turn start does. Answered only once noted, so a mark
// the program makes after this answer falls inside the turn.
func (b *backend) turnStarted(l *link, frame Frame) Frame {
	if !b.liveOn(l, TurnBoundary) {
		return Frame{Error: "no live turn boundary"}
	}
	if frame.Turn == "" {
		return Frame{Error: "a turn start names its turn"}
	}
	if !b.enter() {
		return Frame{Error: errClosing.Error()}
	}
	defer b.leave()
	record := turnRecord{started: boottime.Now()}
	if b.handler.Capture != nil {
		record.boundary = b.handler.Capture()
	}
	b.mu.Lock()
	if _, known := b.turns[frame.Turn]; !known {
		b.turns[frame.Turn] = record
	}
	b.mu.Unlock()
	return Frame{OK: true}
}

// turnEnded is an end's whole handling on one goroutine: its capture, then the
// core's decision.
func (b *backend) turnEnded(l *link, frame Frame) Frame {
	completion, err := b.endOf(l, frame)
	return b.decide(l, completion, frame.Hold, err)
}

// decide hands an end to the core and answers with its decision: "" when it
// was published, the reason to continue with when it was held.
func (b *backend) decide(l *link, completion harness.Completion, holdable bool, err error) Frame {
	if err != nil {
		return Frame{Error: err.Error()}
	}
	reason, err := b.settle(l, completion, holdable)
	if err != nil {
		return Frame{Error: err.Error()}
	}
	return Frame{OK: true, Reason: reason}
}

// endOf is the completion an end frame names, captured when the frame
// arrives. An end sent again under its name is the same completion — the same
// boundary and times — so the core knows it for the end it already answered.
func (b *backend) endOf(l *link, frame Frame) (harness.Completion, error) {
	if !b.liveOn(l, TurnBoundary) {
		return harness.Completion{}, errors.New("no live turn boundary")
	}
	if frame.End == "" || frame.Turn == "" {
		return harness.Completion{}, errors.New("an end names its turn and its event")
	}
	kind, err := endKind(frame.Outcome)
	if err != nil {
		return harness.Completion{}, err
	}
	if !b.enter() {
		return harness.Completion{}, errClosing
	}
	defer b.leave()
	b.mu.Lock()
	defer b.mu.Unlock()
	if known, ok := b.ends[frame.End]; ok {
		return known, nil
	}
	completion := harness.Completion{
		ID: ID + "/" + frame.End, Thread: b.thread, Kind: kind, Text: frame.Text,
		Started: b.turns[frame.Turn].started,
	}
	// Through the run's gate when it has one, so a read acknowledged now
	// lands on one side of this end; a plain capture otherwise.
	if b.handler.EndCapture != nil {
		completion.Boundary, completion.Ended = b.handler.EndCapture()
	}
	if completion.Boundary == nil && b.handler.Capture != nil {
		completion.Boundary = b.handler.Capture()
	}
	if completion.Ended == 0 {
		completion.Ended = boottime.Now()
	}
	b.ends[frame.End] = completion
	return completion, nil
}

func endKind(outcome string) (inbox.Kind, error) {
	switch outcome {
	case OutcomeCompleted:
		return inbox.Finished, nil
	case OutcomeFailed:
		return inbox.Error, nil
	case OutcomeInterrupted:
		return inbox.Stopped, nil
	}
	return "", fmt.Errorf("an end with an outcome the adapter does not know")
}

// settle hands the end to the core: Confirm when the program can continue a
// held end and the wrapper takes confirmations, Publish otherwise. A failed
// attempt is retried as a new call with its own deadline (E3), never by
// stretching the one that failed — and only while the connection the end came
// on still holds a live turn boundary: an attempt under way when it is
// withdrawn runs out, but none starts after.
func (b *backend) settle(l *link, completion harness.Completion, holdable bool) (string, error) {
	if !b.enter() {
		return "", errClosing
	}
	defer b.leave()
	last := errors.New("no live turn boundary")
	for attempt := range endAttempts {
		if attempt > 0 {
			select {
			case <-b.ctx.Done():
				return "", b.ctx.Err()
			case <-l.closed:
			case <-time.After(endBackoff):
			}
		}
		if !b.liveOn(l, TurnBoundary) {
			return "", fmt.Errorf("the turn boundary was withdrawn: %w", last)
		}
		reason, err := b.attempt(completion, holdable)
		if err == nil {
			return reason, nil
		}
		last = err
	}
	return "", fmt.Errorf("the end was not taken after %d attempts: %w", endAttempts, last)
}

func (b *backend) attempt(completion harness.Completion, holdable bool) (string, error) {
	ctx, cancel := context.WithTimeout(b.ctx, b.endWait)
	defer cancel()
	if holdable && b.handler.Confirm != nil {
		return b.handler.Confirm(ctx, completion)
	}
	if b.handler.Publish == nil {
		return "", errors.New("the wrapper takes no ends")
	}
	return "", b.handler.Publish(ctx, completion)
}

// activity takes a reported state while Telemetry is live on the connection
// it came on. A sample that arrives while the probe is still out is dropped:
// it was taken before the capability was proven, and its probe's success
// later must not make it the session's state.
func (b *backend) activity(l *link, frame Frame) {
	if frame.State != "working" && frame.State != "idle" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if l == nil || b.link != l || !b.live[Telemetry] {
		return
	}
	b.telem = telemetry{activity: frame.State, at: time.Now()}
}

// SessionState is what the program has reported, while Telemetry is live.
func (b *backend) SessionState() sessionstate.Snapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	snapshot := sessionstate.Unknown(b.epoch)
	if b.link == nil || !b.live[Telemetry] {
		return snapshot
	}
	now := time.Now()
	snapshot.Selection, snapshot.Thread = "ready", b.thread
	snapshot.Fresh, snapshot.ObservedAt = true, &now
	snapshot.Interruptions = sessionstate.InterruptionsObserved
	if b.telem.activity != "" {
		activity, at := b.telem.activity, b.telem.at
		snapshot.Activity, snapshot.ActivityAt, snapshot.ActivityFresh = &activity, &at, true
	}
	return snapshot
}
