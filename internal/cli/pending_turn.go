package cli

import (
	"github.com/praline-labs/rewake/internal/core/mail"
)

// Who may mark (docs/mail-bridge-turns.md#a-pending-mark-at-its-turns-end).
// A pending mark speaks for one turn, and only an attempt that runs in that
// turn writes it. No absence proves the turn still open — of a later turn's
// start, written in the background, or of an end — so an attempt that cannot
// show it runs in that turn does not mark, and says why.

// inOwnTurn says whether this attempt runs in the turn its operation was made
// in: the attempt that created the record, or a later tool call whose ticket
// names the same conversation and turn on the same transport, where both the
// ticket and the record carry the transport's declaration that its turn ids
// are never reused. Without it only the first attempt counts: a turn id no
// transport declares unique may come back, after an Esc for one.
func inOwnTurn(ctx *Context, op *operation) bool {
	scope := attemptScope(ctx, op)
	return scope.First || scope.InOwnTurn
}

// attemptScope is inOwnTurn's answer in the core's terms: which of the two
// ways the attempt is in its operation's turn, if either.
func attemptScope(ctx *Context, op *operation) mail.AttemptScope {
	if op == nil || op.first {
		return mail.AttemptScope{First: true}
	}
	if ctx.scope == nil {
		return mail.AttemptScope{}
	}
	ticket := ctx.scope.ticket
	record := op.record
	return mail.AttemptScope{InOwnTurn: ticket.TurnsNeverReused && record.TurnsNeverReused && record.Transport == ticket.Transport &&
		record.Turn != "" && record.Conversation == ticket.Conversation && record.Turn == ticket.Turn}
}
