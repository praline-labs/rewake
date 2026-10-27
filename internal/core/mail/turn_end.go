package mail

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The end of a turn as the core publishes it: what a harness's turn boundary
// hands the wrapper ends here, through cli's ReportCompletion and
// ConfirmCompletion (cli/completion.go). The Stop hook called it through the
// hidden turn-ended command until S9.

// CompleteTurn publishes a turn end that may not be held: EndTurn for a
// harness whose boundary cannot hold one open.
func CompleteTurn(parent context.Context, dir string, self registry.Session, event TurnEnd, currentThread string, options TurnEndOptions) (Outcome, error) {
	return EndTurn(parent, dir, self, event, false, currentThread, options)
}

// Outcome is what a turn end the core took said of the work, decided where it
// was taken, for what acts on the end after it: the adapter continues a held
// end, and only a finish is looked at for what its turn left unread
// (Box.Unattended). An end answered from its record — published already, or
// published with the continuation of the end it held — says nothing again.
type Outcome struct {
	// Held is the reason the adapter continues the turn with: the end was
	// held for the session to confirm, and nothing was published.
	Held string
	// Interim says the end was published under a pending mark of its turn:
	// the work goes on.
	Interim bool
	// Finished is the boot-clock end of a turn published as finished, with
	// words, not interim; zero for any other end, and for one of no known
	// time.
	Finished int64
}

// TurnEndOptions is what a caller adds to a turn end beyond the end itself.
type TurnEndOptions struct {
	// BeforeReports runs between reading the waiters and reporting to them;
	// nil does nothing. A test widens that window through it.
	BeforeReports func()
}

// EndTurn publishes a turn end and answers what it said, or holds it for the
// session to confirm (turn_hold.go): then it publishes nothing and answers the
// reason to hand the model. holdable says the end may be held at all, which
// only the harness that reported it can say.
func EndTurn(parent context.Context, dir string, self registry.Session, event TurnEnd, holdable bool, currentThread string, options TurnEndOptions) (Outcome, error) {
	// Only a proof that the name passed on drops the end; a record that
	// could not be read leaves it to be sent again.
	if owned, err := registry.Ownership(dir, self.Name, self.Epoch()); err != nil {
		return Outcome{}, fmt.Errorf("could not read whether this run still owns its name, so the end was not taken: %w", err)
	} else if !owned {
		return Outcome{}, ErrNameLost
	}
	if event.ID != "" && event.Boundary == nil {
		// Its scope could come only from a record written at its first
		// attempt, and a retry after that write failed would fix the scope of
		// its own moment instead (docs/delivery-turn-end.md#the-operation).
		// Its waits stay owed for the next end.
		return Outcome{}, errors.New("a turn end named by an event id carries no read boundary, so its scope is unknown and it reports nothing; its waits stay owed for the next turn end")
	}

	// Under the mailbox lock, so two ends of a turn reported at once tell each
	// waiter once, and a waiter recorded by a read in the meantime is not taken
	// for the one reported.
	ctx, cancel := context.WithTimeout(parent, turnEndLockWait)
	defer cancel()
	var outcome Outcome
	err := state.WithMailboxLock(ctx, dir, self.Name, func() error {
		op := turnOp(self, event)
		if event.ID != "" {
			// An end on record is answered from the record, never run again: a
			// done journal of it; a hold of it, answered with the same reason
			// and nothing kept again; or a journal that took its kept answer,
			// so it was published with its continuation. Only an end of
			// another identity takes a kept answer.
			recorded, err := JournalRecorded(dir, self.Name, op)
			if err != nil || recorded {
				return err
			}
			held, ok, err := HeldEnd(dir, self.Name, self.Epoch(), op)
			if err != nil || ok {
				outcome.Held = held
				return err
			}
			if taken, err := HeldEndTaken(dir, self.Name, self.Epoch(), op); err != nil || taken {
				return err
			}
		}
		waiters, err := scopedWaiters(dir, self.Name, self.Epoch(), event.Boundary)
		if err != nil {
			return err
		}
		mark, err := turnMark(dir, self, event)
		if err != nil {
			return err
		}
		if outcome.Held = holdTurn(dir, self, event, op, holdable, waiters, mark != nil); outcome.Held != "" {
			return nil
		}
		if options.BeforeReports != nil {
			options.BeforeReports()
		}
		outcome, err = publishTurnContext(ctx, dir, self, event, currentThread, waiters, op, mark)
		return err
	})
	if err != nil {
		return Outcome{}, err
	}
	return outcome, nil
}

// ErrNameLost is a turn end of a run proven no longer to own its name — the
// record gone, or a later run's: nothing was taken, and no attempt of the
// same end has anywhere to go.
var ErrNameLost = errors.New("this run no longer owns its name, so its turn end has nowhere to go")

// turnMark finds the pending mark that decides a turn end: this run's latest
// in the end's window (docs/delivery-turn-end.md#pending-marks). Only a
// finish is softened by one, and an end whose time is not known has no
// window. A mark that cannot be read may be this turn's, and stops the end.
func turnMark(dir string, self registry.Session, event TurnEnd) (*Mark, error) {
	if event.Failed || event.Stopped || event.Ended == 0 {
		return nil, nil
	}
	start, err := turnWindowStart(dir, self.Name, self.Epoch(), event.Started, event.Ended)
	if err != nil {
		return nil, err
	}
	mark, ok, err := markWithin(dir, self.Name, self.Epoch(), start, event.Ended)
	if err != nil || !ok {
		return nil, err
	}
	return &mark, nil
}

// turnEndLockWait is how long the end of a turn waits for the mailbox. What it
// could not report stays owed until the next turn ends.
const turnEndLockWait = 5 * time.Second
