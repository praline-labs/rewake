package mail

import (
	"github.com/praline-labs/rewake/internal/core/registry"
)

// MarkVerdict is what a pending mark found under the mailbox lock, just before
// it is written.
type MarkVerdict int

const (
	// VerdictWrite says the turn is not shown ended: it marks.
	VerdictWrite MarkVerdict = iota
	// VerdictTurnEnded says an end is on record at or after the mark's time,
	// or a turn start was recorded after it; no later end's window can hold
	// the mark.
	VerdictTurnEnded
)

// JudgeMark decides a mark under the mailbox lock. started is the latest turn
// start on record, zero when none is. An error writes no mark: a journal that
// cannot be read may be this turn's end.
func JudgeMark(dir string, self registry.Session, epoch string, at, started int64) (MarkVerdict, error) {
	ended, err := endedSince(dir, self.Name, epoch, at)
	if err != nil {
		return 0, err
	}
	// A later start proves the turn ended; its absence proves nothing.
	if ended || started > at {
		return VerdictTurnEnded, nil
	}
	return VerdictWrite, nil
}
