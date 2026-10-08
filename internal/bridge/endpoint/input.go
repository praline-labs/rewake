package endpoint

import (
	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/channel"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/receipt"
)

// The neutral input (docs/v2/stage3-steps-adapters.md, S7):
// the five things the endpoint's state takes from a harness, whichever harness
// reported them. The two parsers of events.go are callers of these, and leave
// with their adapters; what they decode stays theirs.

var _ harness.ToolInput = (*Endpoint)(nil)

// TurnStarted opens a turn a call may be bound to. The table keys a turn by
// its id alone: a run holds one conversation. A start the harness timed is
// recorded in the mailbox first, so a pending mark of an earlier turn whose
// end was lost finds its turn over
// (docs/mail-bridge-turns.md#a-pending-mark-at-its-turns-end).
func (e *Endpoint) TurnStarted(_, turn string, at int64) {
	if at > 0 {
		_ = inbox.RecordTurnStart(e.cfg.Dir, e.cfg.Name, e.cfg.Epoch, at)
	}
	e.turnStarted(turn)
}

// TurnEnded closes it: no call is bound to it after.
func (e *Endpoint) TurnEnded(_, turn string) { e.turnEnded(turn) }

// CallSeen records the harness's own record of a call.
func (e *Endpoint) CallSeen(call harness.ObservedCall) { e.callSeen(call, "", nil) }

// callSeen is CallSeen with what only the hook path carries: the prompt a
// call's turn is named from, and the limits the hook saw.
func (e *Endpoint) callSeen(call harness.ObservedCall, prompt string, limits *HookLimits) {
	digest, refusal := e.observedWords(call.Words, call.Words != nil)
	if call.Nested {
		refusal = "a nested agent's call does not run through the tool"
	}
	e.observe(call.ID, observation{conversation: call.Conversation, turn: call.Turn, prompt: prompt, digest: digest, refusal: refusal, limits: limits})
}

// CallResult hands a call's result to the acknowledgment.
func (e *Endpoint) CallResult(callID string, result harness.ToolResult) {
	e.complete(callID, exposure(callID, result))
}

// StartupFailed fails the channel of the conversation the tool could not
// start for. The harness's own text is not taken: the class is the endpoint's.
func (e *Endpoint) StartupFailed(conversation string) {
	e.tell(channel.Event{Kind: channel.StartupFailed, Thread: conversation})
}

// exposure is what a result proves of one call's answer reaching the model:
// its first text is the answer, and its size is the result's as the encoder
// frames it. A result with no text, or shortened, proves no answer.
func exposure(callID string, r harness.ToolResult) bridge.Exposure {
	evidence := bridge.Exposure{CallID: callID, Direct: r.Direct, Succeeded: r.Succeeded}
	if r.Shortened || len(r.Texts) == 0 {
		evidence.Shortened = true
		return evidence
	}
	rest := ""
	for _, text := range r.Texts[1:] {
		rest += text
	}
	evidence.Answer = []byte(r.Texts[0])
	evidence.ResultBytes = bridge.EncodedSize(r.Texts[0], rest)
	return evidence
}

// complete hands a call's result to the acknowledgment, which runs on its
// own: the event's reader never waits for it. Only a call whose ticket a
// child used can have shown anything, and only its first result is handled:
// the attempt is spent before anything is read or waited for, so a repeat
// cannot retry an acknowledgment the first one dropped
// (docs/mail-bridge-turns.md#acknowledging-a-read).
func (e *Endpoint) complete(id string, evidence bridge.Exposure) {
	c := e.calls
	c.mu.Lock()
	entry := c.byCall[id]
	if entry == nil || entry.completed {
		c.mu.Unlock()
		return
	}
	entry.completed = true
	var ticket bridge.Ticket
	used := entry.issued != nil && entry.issued.used
	if used {
		ticket = entry.issued.ticket
	}
	c.mu.Unlock()
	if !used || e.cfg.Acknowledge == nil {
		return
	}
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return
	}
	e.acks.Add(1)
	e.mu.Unlock()
	go func() {
		defer e.acks.Done()
		token, err := receipt.Bound(e.cfg.Dir, e.cfg.Name, e.cfg.Epoch, bridge.CallKey(ticket.Transport, ticket.Conversation, ticket.CallID))
		if err != nil {
			// Missing or unreadable: which read the call showed is
			// unknown, and a later call shows the letter under its own.
			return
		}
		e.step("acknowledge")
		_ = e.cfg.Acknowledge(e.cfg.Dir, e.cfg.Name, e.cfg.Epoch, token, evidence, e.cfg.Gate)
	}()
}
