package mail

// Step names the step of a mail operation that failed, so its caller can say
// what was left undone without reading the error's words.
type Step int

const (
	// StepLock is the mailbox lock, not taken: nothing was looked at.
	// state.ErrMailboxBusy unwraps from it when another holder kept it.
	StepLock Step = iota + 1
	// StepUnread is the listing of the unread letters.
	StepUnread
	// StepSelect is the letter a read asked for, not among the available
	// ones: unknown, not yet announced, held for an answer or already read.
	StepSelect
	// StepShow is the caller's own showing of what was read: nothing was
	// counted as read.
	StepShow
	// StepMark is the record of a read, after the letters were shown: the
	// one that failed and those after it stay unread.
	StepMark
	// StepOwed is the reading of what this run owes.
	StepOwed
	// StepWaiters is the reading of who waits for this turn.
	StepWaiters
	// StepTurnStart is the reading of the latest turn start on record.
	StepTurnStart
	// StepEnded is the reading of the turn ends on record.
	StepEnded
	// StepPending is the write of the pending mark itself.
	StepPending
)

// StepError is a mail operation that stopped at Step; Err says why.
type StepError struct {
	Step Step
	Err  error
}

func (e *StepError) Error() string { return e.Err.Error() }

func (e *StepError) Unwrap() error { return e.Err }

// failedAt is err as the failure of step, nil when err is.
func failedAt(step Step, err error) error {
	if err == nil {
		return nil
	}
	return &StepError{Step: step, Err: err}
}
