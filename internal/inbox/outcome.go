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
// lock context allows. A lock that cannot be taken leaves fn unrun.
func (s *Server) lock(fn func() error) error {
	return s.lockWithContext(s.lockCtx(), fn)
}

func (s *Server) lockWithContext(ctx context.Context, fn func() error) error {
	return state.WithMailboxLock(ctx, s.Dir, s.Name, fn)
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
// decide at once; nothing that takes a copy away runs here (settle runs
// under lock only).
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
//
// A lock that cannot be taken still lets the status be written, alone; the
// settling waits for a pass that holds the lock (lockOrAlone). It answers
// whether the letter is settled now.
func (s *Server) publish(id string, result Result) bool {
	if last, tried := s.attempts[id]; tried && time.Since(last) < retryInterval && !s.stopping {
		return false
	}
	s.attempts[id] = time.Now()

	settled := false
	err := s.lock(func() error {
		outcome, err := s.recordLocked(id, result)
		if err != nil {
			return err
		}
		settled = s.settleOutcome(id, outcome, result.ReportAvailable)
		return nil
	})
	var unusable *state.LockUnusableError
	if errors.As(err, &unusable) {
		_, _ = s.recordLocked(id, result)
	}
	return settled
}

// record writes what delivery did, unless the agent has read the message
// already, and returns the outcome that stands.
func (s *Server) record(id string, result Result) State {
	var outcome State
	_ = s.lockOrAlone(s.lockCtx(), func() error {
		var err error
		outcome, err = s.recordLocked(id, result)
		return err
	})
	return outcome
}

// recordLocked writes the status in a mailbox section or in the server's
// status-only fallback; it never settles the letter. Read is final: the
// agent has the text, so whatever the harness said about the notice afterwards
// — pending, failed, even delivered — must not undo that, or the task is handed
// out again or its sender told it was refused. Withdrawn is final the same
// way: a late delivered would tell its sender the message arrived after all,
// and a late failed would archive the tombstone its reader is to find.
//
// A letter being read in parts is delivered, whatever the harness says of
// its notice: part of its text is in front of the agent. Recorded as failed,
// its sender would hear it was refused and the settling would take it out of
// unread/ in the middle of the read, which only a read may do.
//
// A status that cannot be read may be final, so nothing is written over it:
// the error leaves the outcome to be recorded on a later pass.
func (s *Server) recordLocked(id string, result Result) (State, error) {
	current, known, err := ReadStatus(s.Dir, s.Name, id)
	if err != nil {
		return "", err
	}
	if known && current.final() {
		s.outcomes[id] = current.result()
		return current.outcome(), nil
	}
	if result.State != Delivered && result.State != Read && claimed(s.Dir, s.Name, id) {
		result = Result{State: Delivered, Via: result.Via, Detail: "being read in parts", GrantApplied: result.GrantApplied}
		s.outcomes[id] = result
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
func (s *Server) settleOutcome(id string, outcome State, reportAvailable bool) bool {
	if outcome == Failed && reportAvailable {
		return settle(s.Dir, s.Name, id, Delivered)
	}
	return settle(s.Dir, s.Name, id, outcome)
}

// settle takes a message with an outcome out of the waiting set. One the harness
// was told about stays readable in unread/ — or has been read already — so only
// the waiting copy goes. A refused one is taken back from unread/ and archived.
// The caller holds the mailbox lock: a server whose lock cannot be taken
// settles nothing (lockOrAlone).
//
// Settling never removes a letter's last copy. The letter is the proof that a
// publication landed while its mark may still say only intent, and retiring
// that proof is the sweep's alone, under the lock and by the live run
// (docs/v2/stage3-publication.md#the-contract), which settling is not. So a
// copy goes only beside another that stands, and a copy whose archive failed
// stays where it is for the next pass.
//
// Nor does it remove or move a copy an open occurrence of the mailbox's stop
// names, or one inside a directory an occurrence names (stop.go): that copy
// is what the occurrence waits to read again, and one settled away would
// leave its stop to an operator. The letter stays where it is, its outcome
// recorded, and a pass after the stop is resolved settles it; letters the
// stop does not name settle as ever.
//
// It answers whether the letter now stands where its outcome puts it. A
// letter it left — named by a stop, or its archive or removal failed — is
// still owed, and the passes find it again by its status and its copies
// (settleRecorded), not by anything remembered here.
func settle(dir, name, id string, outcome State) bool {
	kept := keptByStopOf(dir, name)
	waiting := filepath.Join(state.InboxPath(dir, name), id+".json")
	switch outcome {
	case Held:
		// Neither delivered nor refused yet, so both copies stay: its status
		// keeps it from being announced again, and the readable copy is there
		// for an agent that reads its mail on its own.
		return true
	case Delivered, Read, withdrawn:
		// A withdrawal takes the waiting copy itself; one left behind goes
		// here, and the tombstone stays where its reader will look.
		if kept.keeps(waiting) || !standsIn(id, state.UnreadPath(dir, name), state.DonePath(dir, name)) {
			return gone(waiting)
		}
		if err := removeWaiting(waiting); err == nil {
			_ = state.SyncDir(state.InboxPath(dir, name))
		}
		return gone(waiting)
	default:
		unread := filepath.Join(state.UnreadPath(dir, name), id+".json")
		if claimed(dir, name, id) {
			// Only a read takes a letter being read out of unread/; a failed
			// status an earlier run left behind does not. The read settles it.
			return true
		}
		if kept.keeps(waiting) || kept.keeps(unread) || kept.names(filepath.Join(state.DonePath(dir, name), id+".json")) {
			return false
		}
		// A notice taken back after it was reported delivered has lost its
		// waiting copy; the readable one is then the last, and goes to done/.
		if err := move(id, state.InboxPath(dir, name), state.DonePath(dir, name)); errors.Is(err, os.ErrNotExist) {
			_ = move(id, state.UnreadPath(dir, name), state.DonePath(dir, name))
		}
		if standsIn(id, state.InboxPath(dir, name), state.DonePath(dir, name)) {
			dropUnread(dir, name, id)
		}
		return gone(waiting) && gone(unread)
	}
}

// gone says whether nothing stands at the path. One that cannot be looked at
// may still be there.
func gone(path string) bool {
	_, err := os.Lstat(path)
	return errors.Is(err, os.ErrNotExist)
}

// standsIn says whether a copy of the letter stands in one of the stages:
// a letter, its tombstone, or the link unread/ shares with the waiting copy,
// each a regular file. A directory or a symbolic link at the path is no copy,
// whatever it points at, and neither is a stage that cannot be looked at.
func standsIn(id string, stages ...string) bool {
	for _, stage := range stages {
		if info, err := os.Lstat(filepath.Join(stage, id+".json")); err == nil && info.Mode().IsRegular() {
			return true
		}
	}
	return false
}

// alreadySettled reports whether this message has an outcome, from this run or
// from a previous one whose status or archiving did not complete. A status
// that cannot be read may be one, so the message is left alone for this pass,
// neither delivered again nor refused. Settling it takes the lock; one that
// cannot be taken leaves it for a later pass, which finds it by its waiting
// copy again, at the retry interval.
func (s *Server) alreadySettled(message Message) bool {
	if result, known := s.outcomes[message.ID]; known {
		if result.State == Held {
			s.stillHeld(message, result)
			return true
		}
		s.publish(message.ID, result)
		return true
	}
	status, ok, err := ReadStatus(s.Dir, s.Name, message.ID)
	if err != nil {
		s.attempts[message.ID] = time.Now()
		return true
	}
	if !ok || status.State == Pending || status.State == Held {
		// Held on disk and not in this run's memory was held by a run that
		// died without saying how it ended — a wrapper killed outright. That
		// is no outcome: like pending, the message is still to be decided.
		return false
	}
	if last, tried := s.attempts[message.ID]; tried && time.Since(last) < retryInterval && !s.stopping {
		return true
	}
	s.attempts[message.ID] = time.Now()
	_ = s.lock(func() error {
		s.settleOnRecord(message.ID)
		return nil
	})
	return true
}

// settleOnRecord settles a letter by its status as it stands under the lock
// the caller holds, not as it was read before the lock was taken: a
// withdrawal, a read or a claim that came while the lock was waited for
// decides instead (settle), and a status that no longer reads or no longer
// holds an outcome settles nothing. A letter settled is forgotten; one left
// is tried again once the retry interval has passed.
func (s *Server) settleOnRecord(id string) {
	status, ok, err := ReadStatus(s.Dir, s.Name, id)
	if err != nil || !ok || status.State == Pending || status.State == Held {
		return
	}
	s.outcomes[id] = status.result()
	if s.settleOutcome(id, status.outcome(), status.ReportAvailable) {
		delete(s.attempts, id)
	}
}
