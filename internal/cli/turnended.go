package cli

import (
	"context"
	"errors"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The end of a turn as the core publishes it: what a harness's turn boundary
// hands the wrapper (completion.go) ends here. The Stop hook called
// it through the hidden turn-ended command until S9; S14 moves these to the
// core.

func completeTurn(dir string, self registry.Session, event mail.TurnEnd, currentThread string) error {
	return completeTurnContext(context.Background(), dir, self, event, currentThread, TurnEndOptions{BeforeReports: beforeReports})
}

func completeTurnContext(parent context.Context, dir string, self registry.Session, event mail.TurnEnd, currentThread string, options TurnEndOptions) error {
	_, err := endTurnContext(parent, dir, self, event, false, currentThread, options)
	return err
}

// TurnEndOptions is what a caller adds to a turn end beyond the end itself.
type TurnEndOptions struct {
	// BeforeReports runs between reading the waiters and reporting to them;
	// nil does nothing. A test widens that window through it.
	BeforeReports func()
}

// endTurnContext publishes a turn end, or holds it for the session to confirm
// (turn_hold.go): then it publishes nothing and answers the reason to hand
// the model. holdable says the end may be held at all, which only the
// harness that reported it can say.
func endTurnContext(parent context.Context, dir string, self registry.Session, event mail.TurnEnd, holdable bool, currentThread string, options TurnEndOptions) (string, error) {
	if !registry.OwnsName(dir, self.Name, self.Epoch()) {
		return "", nil
	}
	if event.ID != "" && event.Boundary == nil {
		// Its scope could come only from a record written at its first
		// attempt, and a retry after that write failed would fix the scope of
		// its own moment instead (docs/turn-end-recovery.md#the-operation).
		// Its waits stay owed for the next end.
		return "", errors.New("a turn end named by an event id carries no read boundary, so its scope is unknown and it reports nothing; its waits stay owed for the next turn end")
	}

	// Under the mailbox lock, so two ends of a turn reported at once tell each
	// waiter once, and a waiter recorded by a read in the meantime is not taken
	// for the one reported.
	ctx, cancel := context.WithTimeout(parent, turnEndLockWait)
	defer cancel()
	reason := ""
	err := state.WithMailboxLock(ctx, dir, self.Name, func() error {
		// Every record of the mailbox is reconciled first: a wait an
		// unfinished journal answered would be answered again, and a stopped
		// mailbox changes nothing (docs/turn-end-recovery.md#reconciliation).
		if err := mail.Reconcile(ctx, dir, self.Name); err != nil {
			return err
		}
		op := turnOp(self, event)
		if event.ID != "" {
			// An end on record is answered from the record, never run again: a
			// journal of it, which the barrier has completed; a hold of it,
			// answered with the same reason and nothing kept again; or a
			// journal that took its kept answer, so it was published with its
			// continuation. Only an end of another identity takes a kept
			// answer.
			recorded, err := mail.JournalRecorded(dir, self.Name, op)
			if err != nil || recorded {
				return err
			}
			held, ok, err := mail.HeldEnd(dir, self.Name, self.Epoch(), op)
			if err != nil || ok {
				reason = held
				return err
			}
			if taken, err := mail.HeldEndTaken(dir, self.Name, self.Epoch(), op); err != nil || taken {
				return err
			}
		}
		waiters, err := mail.ScopedWaiters(dir, self.Name, self.Epoch(), event.Boundary)
		if err != nil {
			return err
		}
		mark, err := turnMark(dir, self, event)
		if err != nil {
			return err
		}
		if reason = holdTurn(dir, self, event, op, holdable, waiters, mark != nil); reason != "" {
			return nil
		}
		if options.BeforeReports != nil {
			options.BeforeReports()
		}
		return publishTurnContext(ctx, dir, self, event, currentThread, waiters, op, mark)
	})
	return reason, err
}

// turnMark finds the pending mark that decides a turn end: this run's latest
// in the end's window (docs/turn-end-recovery.md#pending-marks). Only a
// finish is softened by one, and an end whose time is not known has no
// window. A mark that cannot be read may be this turn's, and stops the end.
func turnMark(dir string, self registry.Session, event mail.TurnEnd) (*mail.Mark, error) {
	if event.Failed || event.Stopped || event.Ended == 0 {
		return nil, nil
	}
	start, err := mail.TurnWindowStart(dir, self.Name, self.Epoch(), event.Started, event.Ended)
	if err != nil {
		return nil, err
	}
	mark, ok, err := mail.MarkWithin(dir, self.Name, self.Epoch(), start, event.Ended)
	if err != nil || !ok {
		return nil, err
	}
	return &mark, nil
}

// turnEndLockWait is how long the end of a turn waits for the mailbox. What it
// could not report stays owed until the next turn ends.
const turnEndLockWait = 5 * time.Second

// beforeReports is the BeforeReports every turn end of cli passes. It does
// nothing; a test widens that window through it.
var beforeReports = func() {}
