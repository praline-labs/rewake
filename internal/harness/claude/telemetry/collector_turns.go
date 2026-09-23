package telemetry

import (
	"context"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

// StoppedText is what a stopped outcome says, the same words the Codex side
// sends, so a sender reads one outcome whichever harness its worker runs.
const StoppedText = "the person at the keyboard stopped this turn"

// maxQueuedStops bounds the interruptions waiting to be published. Each one
// takes the mailbox lock; a person pressing Esc faster than that is still one
// person, and an outcome dropped here is published by nobody, as before.
const maxQueuedStops = 16

// ReportTurns takes the handler that publishes a turn outcome. Called before
// Start; without it an interruption still sets activity but reports nothing.
func (c *Collector) ReportTurns(handler harness.CompletionHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.turns = handler
}

// interrupted builds the stopped outcome of a turn the plugin said was
// aborted. It runs on the reading goroutine and does no filesystem work but
// reading the turn's start: the read boundary is captured now, before any
// later read can widen it, and publishing is left to publishStops.
func (c *Collector) interrupted(event Event, thread string) {
	if c.turns.Publish == nil || c.stops == nil {
		return
	}
	completion := harness.Completion{
		Kind:  inbox.Stopped,
		Text:  StoppedText,
		Ended: event.At,
		// The start of the turn, for the pending mark it uses up
		// (docs/turn-outcomes.md): the latest start recorded, as the Stop
		// hook's turn end reads it.
		Started: ReadTurnStart(TurnStartPath(c.path)),
		Thread:  thread,
	}
	if event.Turn != "" {
		// The receipt key: the same interruption reported twice is
		// published once.
		completion.ID = "claude/" + event.Turn
	}
	if c.turns.Capture != nil {
		completion.Boundary = c.turns.Capture()
	}
	select {
	case c.stops <- completion:
	case <-c.done:
	default:
	}
}

// publishStops hands each interruption to the wrapper, one at a time, for as
// long as the collector runs. The turn's end is then recorded as a turn start
// would be, the way `rewake turn-ended` records every end it hears: a pending
// mark made before it cannot be taken by a later turn.
func (c *Collector) publishStops(ctx context.Context) {
	defer c.worker.Done()
	for {
		select {
		case completion := <-c.stops:
			_ = c.turns.Publish(ctx, completion)
			RecordTurnStart(c.path, completion.Ended)
		case <-c.done:
			return
		}
	}
}
