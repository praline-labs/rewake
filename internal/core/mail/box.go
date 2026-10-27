package mail

import (
	"context"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// Box is one run's own mail: the room it lives in, its session's name and the
// run's epoch. What a run does with its own mailbox hangs off it, so a caller
// names the triple once and cannot pair a name with another run's epoch; what
// touches another session's mailbox — a delivery to a recipient, a status, a
// withdrawal — stays a function that names whose mailbox it is. The epoch may
// be empty for what needs only the mailbox (AvailableUnread, ReserveAnswer).
type Box struct {
	dir, name, epoch string
}

// Open is the mail of the run epoch of the session name in the room dir.
func Open(dir, name, epoch string) Box { return Box{dir: dir, name: name, epoch: epoch} }

// BoxOf is the mail of a registered session's own run.
func BoxOf(dir string, session registry.Session) Box { return Open(dir, session.Name, session.Epoch()) }

// Locked is the mailbox while its lock is held by WithLock. What retires or
// moves a letter runs under that lock and takes none of its own, since the
// lock is not reentrant: these are methods of Locked, so a caller outside a
// locked section has no way to call them, which a test over callers used to
// hold.
type Locked struct{ box Box }

// WithLock runs fn under the mailbox lock.
func (b Box) WithLock(ctx context.Context, fn func(Locked) error) error {
	return state.WithMailboxLock(ctx, b.dir, b.name, func() error { return fn(Locked{b}) })
}

// Settle puts a letter where its recorded outcome says; see settleLocked.
func (l Locked) Settle(id string, outcome State) bool {
	return settleLocked(l.box.dir, l.box.name, id, outcome)
}

// SweepFinished retires the finished letters this run's mailbox keeps past
// their time.
func (l Locked) SweepFinished() { sweepFinishedLocked(l.box.dir, l.box.name, l.box.epoch) }

// SweepAwaiting removes the waits of the runs this one replaced.
func (l Locked) SweepAwaiting() { sweepAwaitingLocked(l.box.dir, l.box.name, l.box.epoch) }

// PeekUnread lists the letters announced to this run and not yet read.
func (b Box) PeekUnread() ([]Message, error) { return peekUnread(b.dir, b.name, b.epoch) }

// AvailableUnread lists the unread letters that are not held back for an answer.
func (b Box) AvailableUnread() ([]Message, error) {
	return availableUnread(b.dir, b.name, b.epoch)
}

// MarkRead records that this run read a letter; reports says it owes a report on it.
func (b Box) MarkRead(message Message, reports bool) error {
	return markRead(b.dir, b.name, b.epoch, message, reports)
}

// OwedMessages lists what this run has read and not yet reported on.
func (b Box) OwedMessages() ([]OwedMessage, error) { return owedMessages(b.dir, b.name, b.epoch) }

// Waiters are the waits this run still owes, none where they cannot be read.
func (b Box) Waiters() []Waiter { return waitersOf(b.dir, b.name, b.epoch) }

// ReadWaiters are the waits this run still owes, or why they cannot be read.
func (b Box) ReadWaiters() ([]Waiter, error) { return readWaiters(b.dir, b.name, b.epoch) }

// LatestTurnStart is the start of this run's latest turn; zero when it has none.
func (b Box) LatestTurnStart() (int64, error) { return latestTurnStart(b.dir, b.name, b.epoch) }

// RecordTurnStart records the start of a turn of this run.
func (b Box) RecordTurnStart(started int64) error {
	return recordTurnStart(b.dir, b.name, b.epoch, started)
}

// LastInterim is the text of this run's latest interim report, if it sent one.
func (b Box) LastInterim() (string, bool, error) { return lastInterim(b.dir, b.name, b.epoch) }

// KeptAnswer is the answer kept for this run's question, if one is.
func (b Box) KeptAnswer() (string, bool, error) { return keptAnswer(b.dir, b.name, b.epoch) }

// KeptAnswerThrough is KeptAnswer taken through the given count.
func (b Box) KeptAnswerThrough(through *uint64) (KeptTaken, bool, error) {
	return keptAnswerThrough(b.dir, b.name, b.epoch, through)
}

// AwaitAnswer shows the answer to a question this run sent, waiting for it.
func (b Box) AwaitAnswer(ctx context.Context, question string, show func(Message) error) (Message, bool, error) {
	return awaitAnswer(ctx, b.dir, b.name, b.epoch, question, show)
}

// ReserveAnswer keeps a question's answer from being announced before the
// asker reads it; the release must be deferred through the whole output.
func (b Box) ReserveAnswer(question string) (func(), error) {
	return reserveAnswer(b.dir, b.name, question)
}

// OpenReadClock initializes this run's read-boundary mapping.
func (b Box) OpenReadClock(ctx context.Context) (*ReadClock, error) {
	return openClock(ctx, b.dir, b.name, b.epoch)
}

// MarkWithin is the mark of this run that decides a turn end whose window is
// (start, ended].
func (b Box) MarkWithin(start, ended int64) (Mark, bool, error) {
	return markWithin(b.dir, b.name, b.epoch, start, ended)
}
