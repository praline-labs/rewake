package telemetry

import (
	"context"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
)

// StoppedText is what a stopped outcome says, the same words the Codex side
// sends, so a sender reads one outcome whichever harness its worker runs.
const StoppedText = "the person at the keyboard stopped this turn"

// InterruptedText is what a stopped outcome says when a main session aborted
// the turn with `rewake interrupt`: a person did not stop it, and the sender
// is told who did.
func InterruptedText(by string) string { return by + " interrupted this turn with rewake interrupt" }

// InterruptedLine is the line the session's next notice carries after a main
// interrupted its turn. Claude Code shows the model no trace of an abort, and
// without it the model reads the next message as if nothing had happened
// (docs/remote-control.md).
func InterruptedLine(by string) string {
	return by + " interrupted your previous turn with rewake interrupt."
}

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
	text := StoppedText
	if event.By != "" {
		text = InterruptedText(event.By)
	}
	completion := harness.Completion{
		Kind:  inbox.Stopped,
		Text:  text,
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

// Interrupter names the session that interrupted this session's last turn and
// has not been told of yet, with a mark to hand back to Told; "" when none.
func (c *Collector) Interrupter() (string, uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.interrupter, c.interrupts
}

// Told says the notice carrying the line for mark went out: the model is told
// once. A later interruption has a later mark and stays.
func (c *Collector) Told(mark uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if mark == c.interrupts {
		c.interrupter = ""
	}
}

// noteInterrupter keeps who interrupted the turn that just ended, for the next
// notice; any other turn start or end lays it aside, since the line speaks of
// the previous turn. Called under c.mu.
func (c *Collector) noteInterrupter(event Event) {
	switch {
	case event.Kind == TurnComplete && event.By != "":
		c.interrupter = event.By
		c.interrupts++
	case event.Kind == TurnStart, event.Kind == TurnComplete:
		c.interrupter = ""
	}
}
