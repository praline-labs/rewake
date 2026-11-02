package build

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// Switching a running room to another build
// (docs/rules/state.md#switching-a-running-room). The lease keeps its own
// rule — the build changes only under the exclusive lock with no other
// holder — and a switch adds what makes one succeed and recover: the mark of
// one attempt, which gates every launch's switching path while it is live,
// and the leader's steps around the commit.

// dropPoll is how often a writer that met a dropping switch looks again.
const dropPoll = 50 * time.Millisecond

// SwitchLive says whether a mark still holds the room at now. A mark that is
// only asking holds while its leader runs; from the first drop on something
// may be parked, so a mark holds until its deadline whatever became of its
// leader.
func SwitchLive(mark state.SwitchMark, now time.Time) bool {
	if mark.Past(now) {
		return false
	}
	if mark.Phase == state.PhaseAsking {
		return proc.Alive(mark.Leader, mark.LeaderStart)
	}
	return true
}

// LiveSwitch is the room's mark when one is live.
func LiveSwitch(room string) (state.SwitchMark, bool) {
	mark, err := state.ReadSwitchMark(room)
	if err != nil {
		return state.SwitchMark{}, false
	}
	return mark, SwitchLive(mark, time.Now())
}

// clearStale removes a mark that holds nothing any more, under the lease the
// caller was just granted. One that cannot be removed is left to the next.
func clearStale(room string) {
	mark, err := state.ReadSwitchMark(room)
	if err == nil && !SwitchLive(mark, time.Now()) {
		_ = state.EndSwitch(room, mark)
	}
}

// seen is what a writer found under the shared lock: the room's build, or
// why it could not be read, and the switch that holds the room, if one does.
type seen struct {
	theirs string
	record error
	mark   state.SwitchMark
	live   bool
}

// errGated is a dropping switch that outlasted a writer's bound.
var errGated = errors.New("the room's switch is dropping its holders")

// gate takes the lock shared and reads the build, and while a switch is
// dropping its holders lets go and waits, within the writers' bound, so the
// switch can take the lock exclusive. It answers the build and the live mark
// it found, with the lock held shared — but for a share refused, or errGated.
func gate(lock *state.LeaseLock, room, own string) (seen, error) {
	deadline := time.Now().Add(shareWait)
	for {
		if err := share(lock, room, own); err != nil {
			return seen{}, err
		}
		theirs, record := state.ReadBuildRecord(room)
		mark, live := LiveSwitch(room)
		if !live || mark.Phase != state.PhaseDropping {
			return seen{theirs: theirs, record: record, mark: mark, live: live}, nil
		}
		if err := lock.Unlock(); err != nil {
			return seen{}, err
		}
		if !time.Now().Before(deadline) {
			return seen{}, errGated
		}
		time.Sleep(dropPoll)
	}
}

// Follow takes the room's lease for an image that adopts a run: as a writer,
// never switching the build, and with the room's copy put in line with the
// build when they differ — a switch whose leader died between the build and
// the copy left the copy behind. Adoption never calls AcquireLaunch.
func Follow(base, room string) (*Lease, error) {
	lease, err := Acquire(base, room)
	if err != nil {
		return nil, err
	}
	own, err := ID()
	if err != nil {
		lease.Release()
		return nil, err
	}
	lease.executable, lease.note = pin(lease.room, own)
	return lease, nil
}

// Switch is one attempt to switch the room, led from the lease its leader
// holds shared.
type Switch struct {
	lease *Lease
	Mark  state.SwitchMark
}

// ClaimedError is an attempt refused because another one holds the room.
type ClaimedError struct {
	Room string
	Mark state.SwitchMark
}

func (e *ClaimedError) Error() string {
	return fmt.Sprintf("room %s is already switching to build %s, led by pid %d, so nothing was changed; run it again once that ends.", filepath.Base(e.Room), Short(e.Mark.Target), e.Mark.Leader)
}

// Claim publishes the mark of a new attempt to switch to target, its first
// effect: nothing is staged or bound before the room is the attempt's. A live
// mark of another attempt refuses with ClaimedError; a stale one is replaced.
func Claim(lease *Lease, target, socket string, deadline time.Time) (*Switch, error) {
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		return nil, err
	}
	mark := state.SwitchMark{
		Version: state.SwitchVersion, Leader: os.Getpid(), LeaderStart: start, Target: target,
		Staged: state.StagedPath(lease.room, target), Socket: socket, Phase: state.PhaseAsking, Deadline: deadline.UnixMilli(),
	}
	found, err := state.ClaimSwitch(lease.room, mark, func(m state.SwitchMark) bool { return !SwitchLive(m, time.Now()) })
	if errors.Is(err, state.ErrSwitchClaimed) {
		return nil, &ClaimedError{Room: lease.room, Mark: found}
	}
	if err != nil {
		return nil, err
	}
	state.Step("switch claimed")
	return &Switch{lease: lease, Mark: mark}, nil
}

// Stage places the candidate under its id's name, from the installed file at
// from, keeping it only when its id is the target's.
func (s *Switch) Stage(from string) error {
	if err := state.EnsureSubdir(filepath.Dir(s.Mark.Staged)); err != nil {
		return err
	}
	return state.PlaceExecutable(s.Mark.Staged, from, from, func(temp string) error {
		id, err := FileID(temp)
		if err != nil {
			return err
		}
		if id != s.Mark.Target {
			return errors.New("the installed rewake changed while it was staged")
		}
		return nil
	})
}

// Phase moves the attempt's mark to phase, held until deadline.
func (s *Switch) Phase(phase string, deadline time.Time) error {
	mark := s.Mark
	mark.Phase, mark.Deadline = phase, deadline.UnixMilli()
	if err := state.UpdateSwitch(s.lease.room, mark); err != nil {
		return err
	}
	s.Mark = mark
	state.Step("switch " + phase)
	return nil
}

// Exclusive turns the leader's lease exclusive within wait, as the commands
// still holding it finish. Not taken, the lease is held shared and
// authorized again, and the holders are named.
func (s *Switch) Exclusive(wait time.Duration) ([]Holder, error) {
	lock := s.lease.lock
	deadline := time.Now().Add(wait)
	for {
		taken, err := lock.TryExclusive()
		if err != nil {
			return nil, err
		}
		if taken {
			state.Step("switch exclusive")
			return nil, nil
		}
		if err := s.share(); err != nil {
			return nil, err
		}
		if !time.Now().Before(deadline) {
			return holdersOf(lock), nil
		}
		time.Sleep(switchPause)
	}
}

// share holds the leader's lock shared and authorized under the room's build.
func (s *Switch) share() error {
	if err := s.lease.lock.Share(shareWait); err != nil {
		return err
	}
	return state.Authorize(s.lease.lock)
}

// Commit switches the build under the exclusive lease (state.CommitSwitch),
// the committed mark held until deadline, then holds the lease shared without
// its authorization: no launch takes it exclusive while the parked images
// adopt, and the leader writes nothing more. A lock lost on the way down
// leaves the decision to the mark.
func (s *Switch) Commit(deadline time.Time) error {
	mark := s.Mark
	mark.Deadline = deadline.UnixMilli()
	if err := state.CommitSwitch(s.lease.lock, mark, time.Now()); err != nil {
		return err
	}
	mark.Phase = state.PhaseCommitted
	s.Mark = mark
	_ = s.lease.lock.Share(shareWait)
	state.Revoke(s.lease.lock)
	return nil
}

// Abort turns an attempt that did not commit into an aborting one, held until
// deadline, under the lease taken shared again: the parked images take the
// old build's lease back before any launch may switch the room.
func (s *Switch) Abort(deadline time.Time) error {
	if err := s.share(); err != nil {
		return err
	}
	return s.Phase(state.PhaseAborting, deadline)
}

// End removes the attempt's mark under the leader's lease.
func (s *Switch) End() error { return state.EndSwitch(s.lease.room, s.Mark) }

// Room is the room the attempt switches.
func (s *Switch) Room() string { return s.lease.room }

// Holders are the processes other than the leader holding the room's lease.
func (s *Switch) Holders() []Holder { return holdersOf(s.lease.lock) }
