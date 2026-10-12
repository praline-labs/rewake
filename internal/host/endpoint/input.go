package endpoint

import (
	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/channel"
	"github.com/praline-labs/rewake/internal/core/receipt"
	"github.com/praline-labs/rewake/internal/tool"
)

// The neutral input (docs/v2/stage3-steps-adapters.md, S7):
// the things the endpoint's state takes from a harness, whichever harness
// reported them. What a harness's own records say is decoded in its adapter.

var _ adapter.ToolInput = (*Endpoint)(nil)

// TurnStarted opens a turn a call may be bound to. The table keys a turn by
// its id alone: a run holds one conversation. The host calls it with the
// start the run's TurnBoundary reported, after recording that start itself:
// the record is the boundary's, tool or none
// (docs/mail-bridge-turns.md#a-pending-mark-at-its-turns-end).
func (e *Endpoint) TurnStarted(_, turn string) { e.turnStarted(turn) }

// TurnEnded closes it: no call is bound to it after.
func (e *Endpoint) TurnEnded(_, turn string) { e.turnEnded(turn) }

// CallSeen records the harness's own record of a call.
func (e *Endpoint) CallSeen(call adapter.ObservedCall) {
	digest, refusal := e.observedWords(call.Words, call.Words != nil)
	if call.Nested {
		refusal = "a nested agent's call does not run through the tool"
	}
	e.observe(call.ID, observation{conversation: call.Conversation, turn: call.Turn, digest: digest, refusal: refusal})
}

// CallResult hands a call's result to the acknowledgment.
func (e *Endpoint) CallResult(callID string, result adapter.ToolResult) {
	e.complete(callID, exposure(callID, result))
}

// CallDenied tells the channel record the harness denied our tool for
// permission. The block is the record's; the call's own answer is the
// harness's to the model.
func (e *Endpoint) CallDenied(string) { e.tell(channel.Event{Kind: channel.Denied}) }

// exposure is what a result proves of one call's answer reaching the model:
// its first text is the answer, and its size is the result's as the encoder
// frames it. A result with no text, or shortened, proves no answer.
func exposure(callID string, r adapter.ToolResult) tool.Exposure {
	evidence := tool.Exposure{CallID: callID, Direct: r.Direct, Succeeded: r.Succeeded}
	if r.Shortened || len(r.Texts) == 0 {
		evidence.Shortened = true
		return evidence
	}
	rest := ""
	for _, text := range r.Texts[1:] {
		rest += text
	}
	evidence.Answer = []byte(r.Texts[0])
	evidence.ResultBytes = tool.EncodedSize(r.Texts[0], rest)
	return evidence
}

// complete hands a call's result to the acknowledgment, which runs on its
// own: the event's reader never waits for it. Only a call whose ticket a
// child used can have shown anything, and only its first result is handled:
// the attempt is spent before anything is read or waited for, so a repeat
// cannot retry an acknowledgment the first one dropped
// (docs/mail-bridge-turns.md#acknowledging-a-read).
func (e *Endpoint) complete(id string, evidence tool.Exposure) {
	c := e.calls
	c.mu.Lock()
	entry := c.byCall[id]
	if entry == nil || entry.completed {
		c.mu.Unlock()
		return
	}
	entry.completed = true
	var ticket tool.Ticket
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
		token, err := receipt.Bound(e.cfg.Dir, e.cfg.Name, e.cfg.Epoch, tool.CallKey(ticket.Transport, ticket.Conversation, ticket.CallID))
		if err != nil {
			// Missing or unreadable: which read the call showed is
			// unknown, and a later call shows the letter under its own.
			return
		}
		e.step("acknowledge")
		_ = e.cfg.Acknowledge(e.cfg.Dir, e.cfg.Name, e.cfg.Epoch, token, evidence, e.cfg.Gate)
	}()
}
