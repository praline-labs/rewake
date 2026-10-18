package cli

import (
	"github.com/praline-labs/rewake/internal/core/mail"
)

// Who may mark. A pending mark speaks for one turn, and only an attempt that runs in that
// turn writes it. No absence proves the turn still open — of a later turn's
// start, written in the background, or of an end — so an attempt that cannot
// show it runs in that turn does not mark, and says why.

// inOwnTurn says whether this attempt runs in the turn its operation was made
// in: only the attempt that created the record does, since a shell knows no
// turn to name.
func inOwnTurn(op *operation) bool {
	scope := attemptScope(op)
	return scope.First || scope.InOwnTurn
}

// attemptScope is inOwnTurn's answer in the core's terms: which of the two
// ways the attempt is in its operation's turn, if either.
func attemptScope(op *operation) mail.AttemptScope {
	if op == nil || op.first {
		return mail.AttemptScope{First: true}
	}
	return mail.AttemptScope{}
}
