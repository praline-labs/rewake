package cli

import (
	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
)

// Who may mark (docs/mail-bridge-turns.md#a-pending-mark-at-its-turns-end).
// A pending mark speaks for one turn, and only an attempt that runs in that
// turn writes it. No absence proves the turn still open — of a later turn's
// start, written in the background, or of an end — so an attempt that cannot
// show it runs in that turn does not mark, and says why.

// inOwnTurn says whether this attempt runs in the turn its operation was made
// in: the attempt that created the record, or a later tool call whose ticket
// names the same conversation and turn on a transport whose turn ids are never
// reused. A Claude Code prompt_id is not yet shown never to be reused, Esc
// included, so there only the first attempt counts until stage 3 shows it.
func inOwnTurn(ctx *Context, op *operation) bool {
	scope := attemptScope(ctx, op)
	return scope.First || scope.InOwnTurn
}

// attemptScope is inOwnTurn's answer in the core's terms: which of the two
// ways the attempt is in its operation's turn, if either.
func attemptScope(ctx *Context, op *operation) inbox.AttemptScope {
	if op == nil || op.first {
		return inbox.AttemptScope{First: true}
	}
	if ctx.scope == nil {
		return inbox.AttemptScope{}
	}
	ticket := ctx.scope.ticket
	return inbox.AttemptScope{InOwnTurn: ticket.Transport == bridge.CodexTransport && op.record.Transport == bridge.CodexTransport &&
		op.record.Turn != "" && op.record.Conversation == ticket.Conversation && op.record.Turn == ticket.Turn}
}

// markVerdict is what an attempt found under the mailbox lock, just before
// its mark.
type markVerdict int

const (
	// markWrite: no mark yet, the turn not shown ended, and the attempt in
	// it: it marks.
	markWrite markVerdict = iota
	// markFound: an earlier attempt wrote the mark.
	markFound
	// markTurnEnded: an end on record at or after the mark's time, or a turn
	// start recorded after it; no later end's window can hold the mark.
	markTurnEnded
	// markUnproven: whether the turn is open is unknown to this attempt.
	markUnproven
)

// judgeMark decides an attempt's mark under the mailbox lock. scope says
// where the attempt runs, and started is the latest turn start on record,
// zero when none is. An error leaves the operation open: a mark or a journal
// that cannot be read may be this turn's.
func judgeMark(dir string, self registry.Session, epoch, file string, at int64, scope inbox.AttemptScope, started int64) (markVerdict, error) {
	found, err := inbox.MarkExists(dir, self.Name, epoch, file)
	if err != nil {
		return 0, err
	}
	if found {
		return markFound, nil
	}
	ended, err := inbox.EndedSince(dir, self.Name, epoch, at)
	if err != nil {
		return 0, err
	}
	// A later start proves the turn ended; its absence proves nothing.
	if ended || started > at {
		return markTurnEnded, nil
	}
	if !scope.First && !scope.InOwnTurn {
		return markUnproven, nil
	}
	return markWrite, nil
}
