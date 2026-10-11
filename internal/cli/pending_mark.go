package cli

import (
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
)

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
func judgeMark(dir string, self registry.Session, epoch, file string, at int64, scope mail.AttemptScope, started int64) (markVerdict, error) {
	found, err := mail.MarkExists(dir, self.Name, epoch, file)
	if err != nil {
		return 0, err
	}
	if found {
		return markFound, nil
	}
	ended, err := mail.EndedSince(dir, self.Name, epoch, at)
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
