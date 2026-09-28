package inbox

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// grantAlone narrows a pass to what may share one notice: a message carrying
// a grant goes on its own, so its wait for an idle reader holds back no other
// mail, and its roots and a refusal of its grant, which fails the notice, are
// its own. The pass stops before it; mail after it may still join the group
// while it is reserved, and so reach the reader before the grant does.
func grantAlone(pending []Message) []Message {
	for index, message := range pending {
		if CarriesGrant(message) {
			if index == 0 {
				return pending[:1]
			}
			return pending[:index]
		}
	}
	return pending
}

// joins says whether a message found once a group is reserved may join it:
// not one carrying a grant, whose wait for an idle reader was not asked, and
// nothing beside one.
func joins(group []Message, message Message) bool {
	return !CarriesGrant(message) && (len(group) == 0 || !CarriesGrant(group[0]))
}

// checkGrants checks each grant again before the message becomes readable —
// its directories, and that the main it names did send it — and refuses the
// message whose grant does not pass: a task is never handed over without the
// grant it was sent with. Its sender is told. A grant expired is refused here
// too.
func (s *Server) checkGrants(pending []Message) []Message {
	kept := make([]Message, 0, len(pending))
	for _, message := range pending {
		if CarriesGrant(message) && s.expired(message) {
			// Before the harness is asked whether its reader is idle: a
			// grant that is not going out needs no answer.
			s.failGranted(message, Result{State: Failed, Detail: "expired before the session could take it"})
			continue
		}
		if !CarriesGrant(message) {
			kept = append(kept, message)
			continue
		}
		detail := "this session cannot check a grant"
		if s.CheckGrant != nil {
			err := s.CheckGrant(message)
			if err == nil {
				kept = append(kept, message)
				continue
			}
			if errors.Is(err, ErrNotYet) {
				// The sender's wrapper is there and did not answer: asked
				// again on a later pass, until the message expires.
				s.attempts[message.ID] = time.Now()
				s.record(message.ID, Result{State: Pending, Detail: err.Error()})
				continue
			}
			detail = err.Error()
		}
		s.failGranted(message, Result{State: Failed, Detail: "its grant does not pass: " + detail})
	}
	return kept
}

// failGranted records a failed outcome, and for a message that carries a
// grant tells its sender why: a grant that fails leaves a task nobody works
// on, and its sender may have stopped waiting for the status long ago.
func (s *Server) failGranted(message Message, result Result) {
	s.finish(message, result)
	if result.State == Failed && !result.Withdrawn && CarriesGrant(message) {
		s.tellUndelivered(message, result.Detail)
	}
}

// Settled says whether a task no longer needs what was granted with it: its
// reader has read it and reported on it, or its sender took it back. A task
// delivered and not read yet, or read and not reported on, is not settled.
//
// The status is read before the waits: reading records the wait first and
// the read status after it, so a read status seen here has its wait recorded
// already, and a wait found gone was settled by a report.
func Settled(dir, name, id string) bool {
	status, known := ReadStatus(dir, name, id)
	// Any run's wait counts: until a resumed run has taken over what the run
	// before it owed (adopt.go), the wait is still that run's.
	if owedByAnyRun(dir, name, id) {
		return false
	}
	if known {
		return status.final()
	}
	// The status is swept a day after its outcome; a copy still waiting or
	// readable is still ahead of its reader.
	for _, directory := range []string{state.InboxPath(dir, name), state.UnreadPath(dir, name)} {
		if _, err := os.Stat(filepath.Join(directory, id+".json")); err == nil {
			return false
		}
	}
	return true
}

// TaskOpen says, for the main that granted with a task, whether the task is
// still open: on its way to its reader, delivered and not read, or read and
// owed by a run of the reader — any run, since a run that resumed the
// conversation takes over what the run before it owed (AdoptWaits). found is
// false when nothing of the message is there: a grant is registered before
// its letter is written, and one whose letter never was has no task to close.
//
// Everything read here a worker could write. It can make a task look open
// only, and that keeps its grant as long as not reporting on it would.
func TaskOpen(dir, name, id string) (open, found bool) {
	if !state.ValidName(name) || !safeID(id) {
		return false, false
	}
	if owedByAnyRun(dir, name, id) {
		return true, true
	}
	if status, known := ReadStatus(dir, name, id); known {
		return status.State != Read && status.State != Failed && !status.Withdrawn, true
	}
	for _, directory := range []string{state.InboxPath(dir, name), state.UnreadPath(dir, name)} {
		if _, err := os.Stat(filepath.Join(directory, id+".json")); err == nil {
			return true, true
		}
	}
	if _, err := os.Stat(filepath.Join(state.DonePath(dir, name), id+".json")); err == nil {
		return false, true
	}
	return false, false
}

// owedByAnyRun says whether a wait record of any run of the name lists the
// message, and still stands: a run that ended owes it only within the resume
// window (adopt.go).
func owedByAnyRun(dir, name, id string) bool {
	runs, err := os.ReadDir(state.AwaitingPath(dir, name))
	if err != nil {
		return false
	}
	for _, run := range runs {
		if !run.IsDir() {
			continue
		}
		for _, waiter := range Waiters(dir, name, run.Name()) {
			if owedStands(run.Name(), waiter, id) {
				return true
			}
		}
	}
	return false
}
