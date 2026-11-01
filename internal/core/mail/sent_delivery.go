package mail

import (
	"errors"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
)

// Fate says what a sender can tell of a letter it has just written.
type Fate int

const (
	// FateStatus is the letter's status, as written: Via and Detail are its.
	FateStatus Fate = iota
	// FateRead is a letter read already, which is delivered and then some.
	FateRead
	// FateUnknown is a status that could not be read (Err). It may say
	// delivered or read, whoever holds the name now, so it is taken as
	// pending, which sends nobody to do the work twice.
	FateUnknown
	// FateEnded is a session that ended before the letter was delivered.
	FateEnded
	// FateReplaced is a session that ended and whose name another run took
	// before the letter was delivered. That run refuses it.
	FateReplaced
	// FateAnsweredLongAgo is a letter out of the mailbox with no status: it
	// was answered long enough ago that the result is no longer kept.
	FateAnsweredLongAgo
	// FateNoResult is no result yet within the wait. The session has the
	// letter and will take it.
	FateNoResult
)

// Delivery is what became of a letter just written, as far as its sender can
// tell within its wait.
type Delivery struct {
	// State is Delivered, Pending, Held or Failed.
	State State
	Fate  Fate
	// Via and Detail are the status's, where one says them.
	Via, Detail string
	Err         error
}

// await waits for a status; a test stands in for the delivery server here.
var await = Await

// AwaitSent waits up to wait for the outcome of the letter id just written to
// target's run, and says what became of it. A held or pending status is no
// final answer. Promising a later delivery is honest only while the run the
// letter was written for is still there to make one. A session that ended
// since, or whose name a later run took, leaves the letter with nobody to
// take it. A long wait ends when that run does.
func AwaitSent(dir string, target registry.Session, id string, wait time.Duration) Delivery {
	status, known := await(dir, target.Name, id, wait, recipientRunning(dir, target))
	// Await takes a status it cannot read for none yet, so one more look
	// tells absent from unknown.
	var answered bool
	var unknown error
	if !known {
		status, known, unknown = ReadStatus(dir, target.Name, id)
		known = known && unknown == nil
	}
	if !known && unknown == nil {
		answered, unknown = Answered(dir, target.Name, id)
	}
	if answered {
		// The letter left the mailbox, and a status may have been written
		// after the wait gave up. Absent a moment ago is not absent now, and
		// calling a fresh delivery lost sends the sender to do it twice.
		status, known, unknown = ReadStatus(dir, target.Name, id)
		known = known && unknown == nil
	}
	switch {
	case known && status.State == Read:
		return Delivery{State: Delivered, Fate: FateRead, Via: status.Via}
	case known && status.State != Pending && status.State != Held:
		return Delivery{State: status.State, Via: status.Via, Detail: status.Detail}
	case unknown != nil:
		return Delivery{State: Pending, Fate: FateUnknown, Err: unknown}
	}
	// The name is not enough: it may belong to a session that started after
	// this letter was written, and that session will refuse it.
	current, err := registry.Lookup(dir, target.Name)
	switch {
	case errors.Is(err, registry.ErrNotFound):
		return Delivery{State: Failed, Fate: FateEnded}
	case err == nil && current.Epoch() != target.Epoch():
		return Delivery{State: Failed, Fate: FateReplaced}
	case answered && !known:
		// Pending here would promise a delivery that has happened.
		return Delivery{State: Failed, Fate: FateAnsweredLongAgo}
	case known && status.State == Held:
		return Delivery{State: Held, Via: status.Via, Detail: status.Detail}
	case known && status.Detail != "":
		return Delivery{State: Pending, Detail: status.Detail}
	}
	return Delivery{State: Pending, Fate: FateNoResult}
}

// recipientRunning says whether the run a letter was written for still runs.
// A run in another pid namespace cannot be judged from here and counts as
// running, as everywhere else: its wrapper, which can see it, is the one that
// delivers. A record that cannot be read says nothing either way.
func recipientRunning(dir string, target registry.Session) func() bool {
	return func() bool {
		current, err := registry.LookupReadOnly(dir, target.Name)
		if errors.Is(err, registry.ErrNotFound) {
			return false
		}
		return err != nil || current.Epoch() == target.Epoch()
	}
}
