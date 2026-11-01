package mail

import (
	"context"
	"errors"
	"time"
)

// pendingLockWait bounds the wait for the mailbox, as reading does.
const pendingLockWait = 5 * time.Second

// ErrTurnEnded refuses a pending mark whose turn is shown ended: an end is on
// record at or after the mark's time, or a turn start was recorded after it,
// so no later end's window can hold the mark.
var ErrTurnEnded = errors.New("the turn ended first, so its end reported without this mark")

// ErrNothingOwed refuses a pending mark of a turn that owes nobody a report:
// there is nothing to keep open.
var ErrNothingOwed = errors.New("nothing is owed a report")

// MarkTurnPending marks the running turn of this run, at the boot-clock time
// at its call started, as not the end of the work, and answers the names of
// those who wait for its report; that turn's end tells them the work goes on
// (docs/rules/effects.md, "A pending mark").
//
// Every check runs under the mailbox lock, just before the mark, in this
// order: who waits, the latest turn start, the ends on record. A record that
// cannot be read writes no mark and is a *StepError naming it: a journal may
// be this turn's end, a waiter one the mark would speak to. ErrTurnEnded and
// ErrNothingOwed say why no mark was written. The mark and an end's choice of
// its mark share the lock, so a mark written before the end's journal is that
// end's to take, and one after it is never written.
func (b Box) MarkTurnPending(parent context.Context, text string, at int64) ([]string, error) {
	ctx, cancel := context.WithTimeout(parent, pendingLockWait)
	defer cancel()
	var waiting []string
	err := b.WithLock(ctx, func(Locked) error {
		var err error
		waiting, err = b.markTurnPendingLocked(text, at)
		return err
	})
	if err := lockFailure(err); err != nil {
		return nil, err
	}
	return waiting, nil
}

func (b Box) markTurnPendingLocked(text string, at int64) ([]string, error) {
	waiters, err := readWaiters(b.dir, b.name, b.epoch)
	if err != nil {
		return nil, failedAt(StepWaiters, err)
	}
	started, err := latestTurnStart(b.dir, b.name, b.epoch)
	if err != nil {
		return nil, failedAt(StepTurnStart, err)
	}
	ended, err := endedSince(b.dir, b.name, b.epoch, at)
	if err != nil {
		return nil, failedAt(StepEnded, err)
	}
	// A later start proves the turn ended; its absence proves nothing.
	if ended || started > at {
		return nil, ErrTurnEnded
	}
	if len(waiters) == 0 {
		return nil, ErrNothingOwed
	}
	waiting := make([]string, 0, len(waiters))
	for _, waiter := range waiters {
		waiting = append(waiting, waiter.Name)
	}
	if err := MarkPending(b.dir, b.name, b.epoch, MarkName(at, NewID()), text, at); err != nil {
		return nil, failedAt(StepPending, err)
	}
	return waiting, nil
}
