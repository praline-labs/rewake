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

// handle answers one request or event of the program.
func (b *backend) handle(l *link, frame Frame) {
	switch frame.Op {
	case opTurnStarted:
		_ = l.answer(frame, b.turnStarted(frame))
	case opTurnEnded:
		_ = l.answer(frame, b.turnEnded(frame))
	case opActivity:
		b.activity(frame)
	default:
		if frame.ID != 0 {
			_ = l.answer(frame, Frame{Error: "the adapter does not take " + frame.Op})
		}
	}
}

// turnStarted notes the turn's start on the boot clock and captures its read
// boundary, as Codex's turn start does. Answered only once noted, so a mark
// the program makes after this answer falls inside the turn.
func (b *backend) turnStarted(frame Frame) Frame {
	if _, live := b.isLive(TurnBoundary); !live {
		return Frame{Error: "no live turn boundary"}
	}
	if frame.Turn == "" {
		return Frame{Error: "a turn start names its turn"}
	}
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

// turnEnded hands an end to the core and answers with its decision: "" when
// it was published, the reason to continue with when it was held. An end
// sent again under its name is the same completion — the same boundary and
// times — so the core knows it for the end it already answered.
func (b *backend) turnEnded(frame Frame) Frame {
	if _, live := b.isLive(TurnBoundary); !live {
		return Frame{Error: "no live turn boundary"}
	}
	completion, err := b.completion(frame)
	if err != nil {
		return Frame{Error: err.Error()}
	}
	reason, err := b.settle(completion, frame.Hold)
	if err != nil {
		return Frame{Error: err.Error()}
	}
	return Frame{OK: true, Reason: reason}
}

func (b *backend) completion(frame Frame) (harness.Completion, error) {
	if frame.End == "" || frame.Turn == "" {
		return harness.Completion{}, errors.New("an end names its turn and its event")
	}
	kind, err := endKind(frame.Outcome)
	if err != nil {
		return harness.Completion{}, err
	}
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
// stretching the one that failed.
func (b *backend) settle(completion harness.Completion, holdable bool) (string, error) {
	var last error
	for attempt := range endAttempts {
		if attempt > 0 {
			select {
			case <-b.ctx.Done():
				return "", b.ctx.Err()
			case <-time.After(endBackoff):
			}
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

// activity takes a reported state while Telemetry is live.
func (b *backend) activity(frame Frame) {
	if frame.State != "working" && frame.State != "idle" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.link == nil || !b.live[Telemetry] {
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
