package inbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// shutdownLockWait is how long the last writes wait for the mailbox lock.
//
// The workflow suite's termination budget (test/workflow) is the sum of this and
// the other shutdown stages, so a change here has to be reflected there.
const shutdownLockWait = 2 * time.Second

// lock runs fn under the mailbox lock, waiting no longer than the server's
// lock context allows.
//
// A lock nobody can take — its file unopenable, say — does not stop the
// server: fn runs without it. That is safe because every other user of the
// lock fails on it too and says so, which leaves the server the only writer.
// Stopping instead left senders with a pending that explained nothing and a
// status that could not be written.
func (s *Server) lock(fn func() error) error {
	ctx := s.lockContext
	if ctx == nil {
		ctx = context.Background()
	}
	return s.lockWithContext(ctx, fn)
}

func (s *Server) lockWithContext(ctx context.Context, fn func() error) error {
	err := state.WithMailboxLock(ctx, s.Dir, s.Name, fn)
	var unusable *state.LockUnusableError
	if errors.As(err, &unusable) {
		return fn()
	}
	return err
}

// finish records the outcome and takes the message out of the waiting set.
//
// The order matters and so does remembering the outcome. Archiving can fail —
// a full disk, a done/ directory somebody replaced with a file — and a message
// left in the mailbox with nothing remembered about it is delivered again on
// the next pass, four times a second, long after its sender was told it landed.
func (s *Server) finish(message Message, result Result) {
	s.outcomes[message.ID] = result
	delete(s.attempts, message.ID)
	s.publish(message.ID, result)
}

// publish writes the outcome down and takes the message out of the waiting set.
// Both steps are retried on later passes until they hold: a message whose
// outcome is known is never delivered again, only recorded again.
//
// The retry waits, though. Writing the status is itself a change to the mailbox,
// and a directory that cannot be archived into turned that into a loop: write,
// event, pass, write again, hundreds of times a second.
func (s *Server) publish(id string, result Result) {
	if last, tried := s.attempts[id]; tried && time.Since(last) < retryInterval && !s.stopping {
		return
	}
	s.attempts[id] = time.Now()

	_ = s.lock(func() error {
		outcome, err := s.recordLocked(id, result)
		if err != nil {
			return err
		}
		s.settleOutcome(id, outcome, result.ReportAvailable)
		return nil
	})
}

// record writes what delivery did, unless the agent has read the message
// already, and returns the outcome that stands.
func (s *Server) record(id string, result Result) State {
	var outcome State
	_ = s.lock(func() error {
		var err error
		outcome, err = s.recordLocked(id, result)
		return err
	})
	return outcome
}

// recordLocked is record under a lock the caller holds. Read is final: the
// agent has the text, so whatever the harness said about the notice afterwards
// — pending, failed, even delivered — must not undo that, or the task is handed
// out again or its sender told it was refused. Withdrawn is final the same
// way: a late delivered would tell its sender the message arrived after all,
// and a late failed would archive the tombstone its reader is to find.
func (s *Server) recordLocked(id string, result Result) (State, error) {
	if current, ok := ReadStatus(s.Dir, s.Name, id); ok && current.final() {
		s.outcomes[id] = current.result()
		return current.outcome(), nil
	}
	if err := writeStatus(s.Dir, s.Name, id, result); err != nil {
		return "", err
	}
	return result.State, nil
}

// removeWaiting drops the waiting copy of an announced message; replaceable in
// tests, which need that step to fail.
var removeWaiting = os.Remove

// The durable flag distinguishes a failed wake-up from expired or foreign
// mail. Recovery never manufactures an unread copy for an old failed status.
func (s *Server) settleOutcome(id string, outcome State, reportAvailable bool) {
	if outcome == Failed && reportAvailable {
		settle(s.Dir, s.Name, id, Delivered)
		return
	}
	settle(s.Dir, s.Name, id, outcome)
}

// settle takes a message with an outcome out of the waiting set. One the harness
// was told about stays readable in unread/ — or has been read already — so only
// the waiting copy goes. A refused one is taken back from unread/ and archived.
func settle(dir, name, id string, outcome State) {
	switch outcome {
	case Held:
		// Neither delivered nor refused yet, so both copies stay: its status
		// keeps it from being announced again, and the readable copy is there
		// for an agent that reads its mail on its own.
	case Delivered, Read, withdrawn:
		// A withdrawal takes the waiting copy itself; one left behind goes
		// here, and the tombstone stays where its reader will look.
		waiting := filepath.Join(state.InboxPath(dir, name), id+".json")
		if err := removeWaiting(waiting); err == nil {
			_ = state.SyncDir(state.InboxPath(dir, name))
		}
	default:
		// A notice taken back after it was reported delivered has lost its
		// waiting copy; the readable one is then the last, and goes to done/.
		if err := move(id, state.InboxPath(dir, name), state.DonePath(dir, name)); errors.Is(err, os.ErrNotExist) {
			_ = move(id, state.UnreadPath(dir, name), state.DonePath(dir, name))
		}
		dropUnread(dir, name, id)
	}
}

// alreadySettled reports whether this message has an outcome, from this run or
// from a previous one whose status or archiving did not complete.
func (s *Server) alreadySettled(message Message) bool {
	if result, known := s.outcomes[message.ID]; known {
		if result.State == Held {
			s.stillHeld(message, result)
			return true
		}
		s.publish(message.ID, result)
		return true
	}
	status, ok := ReadStatus(s.Dir, s.Name, message.ID)
	if !ok || status.State == Pending || status.State == Held {
		// Held on disk and not in this run's memory was held by a run that
		// died without saying how it ended — a wrapper killed outright. That
		// is no outcome: like pending, the message is still to be decided.
		return false
	}
	s.outcomes[message.ID] = status.result()
	_ = s.lock(func() error {
		s.settleOutcome(message.ID, status.outcome(), status.ReportAvailable)
		return nil
	})
	return true
}
