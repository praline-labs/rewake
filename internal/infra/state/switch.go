package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The switch of a running room to another build
// (docs/rules/state.md#switching-a-running-room): the mark one attempt
// claims, which the room's writers and launches read before their switching
// path, and the commit that changes the build under the exclusive lease.
// infra/build decides when a mark is live; this package keeps the file.

const (
	switchMarkName = "switch"
	switchLockName = ".switch.lock"
	// stagedPrefix names a staged copy beside the room's own:
	// bin/rewake.<build id>, so a copy never changes under its name.
	stagedPrefix = "rewake."
)

// The phases of a switch, in their order; an attempt ends in committed or
// aborting.
const (
	PhaseAsking    = "asking"
	PhaseDropping  = "dropping"
	PhaseCommitted = "committed"
	PhaseAborting  = "aborting"
)

// SwitchVersion is the version of the mark, of the leader's socket messages
// and of a parked image's handover: the switch contract. 2 added the
// attempt's own id.
const SwitchVersion = 2

// ErrNoSwitchMark is a room no switch is marked in.
var ErrNoSwitchMark = errors.New("no switch is marked in the room")

// ErrSwitchClaimed is a claim refused: another attempt's mark is there.
var ErrSwitchClaimed = errors.New("a switch is already marked in the room")

// SwitchMark is one attempt to switch a room's build, written by its leader.
type SwitchMark struct {
	Version int `json:"version"`
	// Leader and LeaderStart are the leader's pid and start, and Attempt the
	// attempt's own id: a leader that tries again claims a new attempt, which
	// a participant that answered the last one joins again.
	Leader      int    `json:"leader"`
	LeaderStart uint64 `json:"leaderStart"`
	Attempt     string `json:"attempt"`
	Target      string `json:"target"`
	Staged      string `json:"staged"`
	Socket      string `json:"socket"`
	Phase       string `json:"phase"`
	// Deadline is in Unix milliseconds.
	Deadline int64 `json:"deadline"`
}

// Same says two marks are one attempt.
func (m SwitchMark) Same(other SwitchMark) bool {
	return m.Leader == other.Leader && m.LeaderStart == other.LeaderStart && m.Attempt == other.Attempt && m.Target == other.Target
}

// Past says the mark's deadline has passed at now.
func (m SwitchMark) Past(now time.Time) bool { return now.UnixMilli() >= m.Deadline }

// SwitchMarkPath is the room's mark.
func SwitchMarkPath(room string) string { return filepath.Join(room, switchMarkName) }

// StagedPath is where a switch stages the build id, beside the room's copy.
func StagedPath(room, id string) string { return filepath.Join(room, "bin", stagedPrefix+id) }

// ReadSwitchMark reads the room's mark; ErrNoSwitchMark when there is none.
// It writes nothing: a view may read it.
func ReadSwitchMark(room string) (SwitchMark, error) {
	data, err := os.ReadFile(SwitchMarkPath(room))
	if errors.Is(err, os.ErrNotExist) {
		return SwitchMark{}, ErrNoSwitchMark
	}
	if err != nil {
		return SwitchMark{}, err
	}
	var mark SwitchMark
	if err := json.Unmarshal(data, &mark); err != nil || mark.Version != SwitchVersion {
		return SwitchMark{}, fmt.Errorf("the switch mark of %s cannot be read", room)
	}
	return mark, nil
}

// withSwitchLock orders the changes of the mark made under the shared lease:
// a claim, a leader's phase, the removal of one that went stale. It is taken
// after the lease, and nothing is taken inside it.
func withSwitchLock(room string, fn func() error) error {
	return withLock(filepath.Join(room, switchLockName), "the room's switch", fn)
}

// ClaimSwitch publishes the mark of a new attempt, under the leaser's lease.
// A mark already there is passed to stale: one it calls stale is removed and
// the claim made; any other refuses with ErrSwitchClaimed and the mark found.
func ClaimSwitch(room string, mark SwitchMark, stale func(SwitchMark) bool) (SwitchMark, error) {
	data, err := json.Marshal(mark)
	if err != nil {
		return SwitchMark{}, err
	}
	var found SwitchMark
	err = withSwitchLock(room, func() error {
		current, err := ReadSwitchMark(room)
		switch {
		case err == nil && !stale(current):
			found = current
			return ErrSwitchClaimed
		case err == nil, !errors.Is(err, ErrNoSwitchMark):
			// A stale mark, or one that cannot be read, holds nothing.
			if err := Remove(SwitchMarkPath(room)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		return PublishExclusive(SwitchMarkPath(room), data)
	})
	return found, err
}

// UpdateSwitch rewrites the attempt's mark with its new phase and deadline,
// under the leader's lease; a mark no longer the attempt's refuses.
func UpdateSwitch(room string, mark SwitchMark) error {
	data, err := json.Marshal(mark)
	if err != nil {
		return err
	}
	return withSwitchLock(room, func() error {
		current, err := ReadSwitchMark(room)
		if err != nil || !current.Same(mark) {
			return fmt.Errorf("the switch of %s is no longer this attempt's", room)
		}
		return WriteAtomic(SwitchMarkPath(room), data)
	})
}

// EndSwitch removes the mark when it is still the given attempt's: the
// leader's end of an attempt that did not commit, or a writer's removal of a
// mark it found stale.
func EndSwitch(room string, mark SwitchMark) error {
	return withSwitchLock(room, func() error {
		current, err := ReadSwitchMark(room)
		if errors.Is(err, ErrNoSwitchMark) || (err == nil && !current.Same(mark)) {
			return nil
		}
		if err := Remove(SwitchMarkPath(room)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	})
}

// CommitSwitch switches the room to the mark's target under the exclusive
// lease this lock holds, in an order a crash at any step leaves sound: the
// mark is written committed, with its deadline, before the build, so a
// launch stays gated whatever is written after it; the build is the commit
// point; then the staged copy takes the room's copy's name, and the copies
// no mark names go. It refuses unless the lock is held exclusive and the
// room's mark is this attempt's, dropping and not past its deadline.
func CommitSwitch(l *LeaseLock, mark SwitchMark, now time.Time) error {
	if !l.held || !l.exclusive {
		return fmt.Errorf("the switch of %s is committed only under the exclusive lease", l.room)
	}
	current, err := ReadSwitchMark(l.room)
	if err != nil || !current.Same(mark) || current.Phase != PhaseDropping || current.Past(now) {
		return fmt.Errorf("the switch of %s is no longer this attempt's to commit", l.room)
	}
	mark.Phase = PhaseCommitted
	data, err := json.Marshal(mark)
	if err != nil {
		return err
	}
	path := SwitchMarkPath(l.room)
	if err := guard(OpWrite, path); err != nil {
		return err
	}
	if err := replace(path, data, nil); err != nil {
		return err
	}
	Step("switch marked")
	if err := writeBuild(l, mark.Target); err != nil {
		return err
	}
	Step("switch build")
	bin := filepath.Join(l.room, "bin")
	copyPath := filepath.Join(bin, "rewake")
	if err := guard(OpWrite, copyPath); err != nil {
		return err
	}
	temp, err := linkTemp(bin, mark.Staged)
	if err != nil {
		return err
	}
	if err := placeTemp(temp, copyPath); err != nil {
		return err
	}
	Step("switch linked")
	// The copies of earlier attempts: no live mark names them, this one being
	// the room's only mark.
	entries, _ := os.ReadDir(bin)
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, stagedPrefix) && filepath.Join(bin, name) != mark.Staged {
			_ = Remove(filepath.Join(bin, name))
		}
	}
	return nil
}

// Revoke ends the authorization this lock gave without letting go of the
// lock: a switch's leader keeps the room's lease shared after its commit, so
// no launch takes it exclusive while the participants adopt, and writes
// nothing more.
func Revoke(l *LeaseLock) { revoke(l) }
