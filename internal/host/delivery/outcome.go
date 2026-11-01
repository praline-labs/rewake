package delivery

import (
	"context"
	"errors"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// shutdownLockWait is how long the last writes wait for the mailbox lock.
//
// The workflow suite's termination budget (test/workflow) is the sum of this and
// the other shutdown stages, so a change here has to be reflected there.
const shutdownLockWait = 2 * time.Second

func (s *Server) lockWithContext(ctx context.Context, fn func() error) error {
	return state.WithMailboxLock(ctx, s.Dir, s.Name, fn)
}

// box is this server's run's own mail.
func (s *Server) box() mail.Box { return mail.Open(s.Dir, s.Name, s.Epoch) }

// locked runs fn under the mailbox lock, waiting no longer than the server's
// lock context allows; a lock that cannot be taken leaves fn unrun. What
// retires or moves a letter runs here, since only mail.Locked can do it.
func (s *Server) locked(fn func(mail.Locked) error) error {
	return s.box().WithLock(s.lockCtx(), fn)
}

func (s *Server) lockCtx() context.Context {
	if s.lockContext == nil {
		return context.Background()
	}
	return s.lockContext
}

// lockOrAlone is lockWithContext for what moves and removes nothing: a
// status, the link that makes a letter readable, a look. A lock whose file
// cannot be opened or locked does not stop those: fn runs without it.
// Stopping instead left senders with a pending that explained nothing and a
// status that could not be written.
//
// That an open fails proves nobody else holds the lock no more than it
// proves anything of a descriptor opened earlier: a holder from before the
// file's mode changed keeps its lock. So what fn decides alone, another may
// decide at once; nothing that takes a copy away runs here (mail.Locked runs
// under the lock only).
func (s *Server) lockOrAlone(ctx context.Context, fn func() error) error {
	err := s.lockWithContext(ctx, fn)
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
func (s *Server) finish(message mail.Message, result mail.Result) {
	s.outcomes[message.ID] = result
	delete(s.attempts, message.ID)
	s.publish(message.ID, result)
}

// publish writes the outcome down and takes the message out of the waiting set
// (mail.Locked.SettleWith). Both steps are retried on later passes until they
// hold: a message whose outcome is known is never delivered again, only
// recorded again.
//
// The retry waits, though. Writing the status is itself a change to the mailbox,
// and a directory that cannot be archived into turned that into a loop: write,
// event, pass, write again, hundreds of times a second.
//
// A lock that cannot be taken still lets the status be written, alone; the
// settling waits for a pass that holds the lock (lockOrAlone). It answers
// whether the letter is settled now.
func (s *Server) publish(id string, result mail.Result) bool {
	if last, tried := s.attempts[id]; tried && time.Since(last) < retryInterval && !s.stopping {
		return false
	}
	s.attempts[id] = time.Now()

	settled := false
	err := s.locked(func(l mail.Locked) error {
		recorded, err := l.SettleWith(id, result)
		if err != nil {
			return err
		}
		s.remember(id, recorded)
		settled = recorded.Settled
		return nil
	})
	var unusable *state.LockUnusableError
	if errors.As(err, &unusable) {
		_, _ = s.recordAlone(id, result)
	}
	return settled
}

// record writes what delivery did, unless the agent has read the message
// already or its sender took it back, and answers whether such a final
// status stands instead.
func (s *Server) record(id string, result mail.Result) bool {
	kept := false
	_ = s.lockOrAlone(s.lockCtx(), func() error {
		recorded, err := s.recordAlone(id, result)
		kept = recorded.Kept
		return err
	})
	return kept
}

// recordAlone writes the status, in a mailbox section or without one; it
// never settles the letter (mail.Box.Record).
func (s *Server) recordAlone(id string, result mail.Result) (mail.Recorded, error) {
	recorded, err := s.box().Record(id, result)
	if err == nil {
		s.remember(id, recorded)
	}
	return recorded, err
}

// remember takes a final status found on record for the outcome, in place of
// what this run thought it was.
func (s *Server) remember(id string, recorded mail.Recorded) {
	if recorded.Kept {
		s.outcomes[id] = recorded.Result
	}
}

// alreadySettled reports whether this message has an outcome, from this run or
// from a previous one whose status or archiving did not complete. A status
// that cannot be read may be one, so the message is left alone for this pass,
// neither delivered again nor refused. Settling it takes the lock; one that
// cannot be taken leaves it for a later pass, which finds it by its waiting
// copy again, at the retry interval.
func (s *Server) alreadySettled(message mail.Message) bool {
	if result, known := s.outcomes[message.ID]; known {
		if result.State == mail.Held {
			s.stillHeld(message, result)
			return true
		}
		s.publish(message.ID, result)
		return true
	}
	status, ok, err := mail.ReadStatus(s.Dir, s.Name, message.ID)
	if err != nil {
		s.attempts[message.ID] = time.Now()
		return true
	}
	if !ok || !status.Decided() {
		return false
	}
	if last, tried := s.attempts[message.ID]; tried && time.Since(last) < retryInterval && !s.stopping {
		return true
	}
	s.attempts[message.ID] = time.Now()
	_ = s.locked(func(l mail.Locked) error {
		s.settleOnRecord(l, message.ID)
		return nil
	})
	return true
}

// settleOnRecord settles a letter by its status as it stands under the lock
// the caller holds (mail.Locked.SettleOnRecord). A letter settled is
// forgotten; one left is tried again once the retry interval has passed.
func (s *Server) settleOnRecord(l mail.Locked, id string) {
	recorded, decided, err := l.SettleOnRecord(id)
	if err != nil || !decided {
		return
	}
	s.outcomes[id] = recorded.Result
	if recorded.Settled {
		delete(s.attempts, id)
	}
}
