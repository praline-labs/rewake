package adapter

import (
	"context"
	"fmt"

	"github.com/praline-labs/rewake/internal/core/mail"
)

// EndReason is why a turn ended, as the adapter's end event says
// (docs/v2/design-api.md#turnboundary). The host decides from it what the end
// reports (docs/rules/turns.md, O2); the adapter does not.
type EndReason string

// The reasons an end can carry.
const (
	// Answered is a turn that ran to its final answer.
	Answered EndReason = "answered"
	// Aborted is a turn stopped before its answer: by the person, or by a
	// main's interrupt.
	Aborted EndReason = "aborted"
	// Refused is a turn the model declined to answer.
	Refused EndReason = "refused"
	// Failed is a turn the harness could not finish: an error of its own or
	// of the model's service.
	Failed EndReason = "failed"
)

// Valid says the reason is one of the four. An end with any other reason is
// refused rather than read as one of them.
func (r EndReason) Valid() bool {
	switch r {
	case Answered, Aborted, Refused, Failed:
		return true
	}
	return false
}

// TurnEnd is one end of a turn of the session's own conversation, as a
// TurnBoundary reports it.
type TurnEnd struct {
	// Event names the end, once and for good: the same end reported again
	// carries the same event, and everything the end writes is named from
	// it (docs/rules/effects.md, E7). An end without one is refused.
	Event string
	// Reason is why the turn ended.
	Reason EndReason
	// Answer is the turn's final text: the answer, or what the harness said
	// of its failure, unchanged.
	Answer string
	// Thread is the conversation the turn ran in, empty where unknown.
	Thread string
	// Boundary is what had been read when the end was heard: the end's
	// scope, fixed before its first effect. An end without one is refused.
	Boundary *mail.ReadBoundary
	// Started and Ended bound the turn on the boot clock (internal/infra/boottime),
	// zero where unknown; a pending mark counts for this turn only if it was
	// made between them.
	Started, Ended int64
}

// Check refuses an end the core cannot take: one without its event, its scope
// or a known reason.
func (e TurnEnd) Check() error {
	switch {
	case e.Event == "":
		return fmt.Errorf("a turn end names its event")
	case e.Boundary == nil:
		return fmt.Errorf("a turn end carries its captured read boundary")
	case !e.Reason.Valid():
		return fmt.Errorf("a turn end carries a reason the core knows, not %q", e.Reason)
	}
	return nil
}

// TurnHandler is what the host gives a TurnBoundary before the run starts:
// where a turn's start goes, how to capture an end's scope without blocking,
// and where the end goes.
type TurnHandler struct {
	// Started takes a turn's start, whatever else the run serves: the
	// conversation and turn it opens, and at, when the turn started on the
	// boot clock — a time the harness took, at or before anything the turn
	// ran, never the time its report arrived; 0 where the harness gives
	// none. The host records a timed start, the evidence a pending mark of
	// an earlier turn whose end was lost is judged by, and opens the turn
	// for the mail tool's calls where the run has a tool. Call it before the
	// start is answered, so whatever the turn does next is after it.
	Started func(conversation, turn string, at int64)
	Capture func() *mail.ReadBoundary
	Publish func(context.Context, TurnEnd) error
	// Confirm runs an end that the adapter can hold open, in place of
	// Publish (docs/v2/design-api.md#turnboundary): it answers "" when the
	// end was published, or the reason to continue the turn with when it was
	// held and its answer kept. The adapter continues the turn with that
	// reason and confirms its next end again. The same end confirmed again
	// gets the same answer. Nil where the wrapper was given none.
	Confirm func(context.Context, TurnEnd) (string, error)
	// EndCapture captures the boundary of a turn's end through the gate
	// the mail tool's acknowledgments enter
	// (docs/mail-bridge-turns.md#a-turns-end-meets-its-calls), with the
	// moment it noted the end on the boot clock. Nil where the run has no
	// gate; Capture is then the end's boundary.
	EndCapture func() (*mail.ReadBoundary, int64)
	// Tool takes what the harness reports of the mail tool's calls in no
	// harness's terms: a turn's end, calls seen, their results. Starts are
	// not among them: they go to Started, tool or none.
	// It never waits. Nil where the run has no tool.
	Tool ToolInput
}

// TurnBoundary is a run's report of its turns: their starts reach the host
// through the handler's Started, their ends through Publish or Confirm,
// whether or not the run has a tool. The host hands it the handler before the
// run's service starts.
type TurnBoundary interface {
	ReportTurns(TurnHandler)
}
