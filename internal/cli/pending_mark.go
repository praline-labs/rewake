package cli

import (
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
)

// MarkVerdict is what an attempt found under the mailbox lock, just before
// its mark.
type MarkVerdict int

const (
	// VerdictWrite says there is no mark yet, the turn is not shown ended and
	// the attempt runs in it: it marks.
	VerdictWrite MarkVerdict = iota
	// VerdictFound says an earlier attempt wrote the mark.
	VerdictFound
	// VerdictTurnEnded says an end is on record at or after the mark's time,
	// or a turn start was recorded after it; no later end's window can hold
	// the mark.
	VerdictTurnEnded
	// VerdictUnproven says whether the turn is open is unknown to this attempt.
	VerdictUnproven
)

// JudgeMark decides an attempt's mark under the mailbox lock. scope says
// where the attempt runs, and started is the latest turn start on record,
// zero when none is. An error leaves the operation open: a mark or a journal
// that cannot be read may be this turn's.
func JudgeMark(dir string, self registry.Session, epoch, file string, at int64, scope mail.AttemptScope, started int64) (MarkVerdict, error) {
	found, err := mail.MarkExists(dir, self.Name, epoch, file)
	if err != nil {
		return 0, err
	}
	if found {
		return VerdictFound, nil
	}
	ended, err := mail.EndedSince(dir, self.Name, epoch, at)
	if err != nil {
		return 0, err
	}
	// A later start proves the turn ended; its absence proves nothing.
	if ended || started > at {
		return VerdictTurnEnded, nil
	}
	if !scope.First && !scope.InOwnTurn {
		return VerdictUnproven, nil
	}
	return VerdictWrite, nil
}
