package build

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// shareWait bounds a writer's wait for the shared lease: an exclusive holder
// keeps it for a write and a sync, and one that stalls refuses its writers
// rather than holding them.
var shareWait = 10 * time.Second

// Lease is a room's build lease this process holds shared, its build checked.
// Release ends it; until then the room is authorized for this process's
// writes.
type Lease struct {
	room string

	mu   sync.Mutex
	lock *state.LeaseLock
	// holds counts the owner's hold and those Keep added; the lock goes with
	// the last.
	holds int
	owner sync.Once
}

// Room is the room the lease holds.
func (l *Lease) Room() string { return l.room }

// Release ends the owner's hold: the lease and the room's authorization for
// this process end with it, or with the last hold Keep added, whichever comes
// later. A nil lease releases nothing; a second Release does nothing.
func (l *Lease) Release() {
	if l == nil {
		return
	}
	l.owner.Do(l.drop)
}

// Keep adds a hold on the lease for a part of the process whose effects may
// outlast its owner's use — a run's workers past its stop — and returns what
// ends that hold, once however often it is called. A lease already released
// keeps nothing.
func (l *Lease) Keep() (release func()) {
	if l == nil {
		return func() {}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.lock == nil {
		return func() {}
	}
	l.holds++
	var once sync.Once
	return func() { once.Do(l.drop) }
}

func (l *Lease) drop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.holds--
	if l.holds == 0 && l.lock != nil {
		l.lock.Close()
		l.lock = nil
	}
}

// UnusableError is a state directory the lease cannot be taken in: a base
// that is not absolute, a room name that is not one, a level somebody else
// could write. It is about the call's environment, not about the room's
// build.
type UnusableError struct{ Err error }

func (e *UnusableError) Error() string { return e.Err.Error() }
func (e *UnusableError) Unwrap() error { return e.Err }

// RefusedError is an acquisition the room's build refused: another build's
// writers, a record that is missing or cannot be used, holders of the lease a
// launch had to take exclusive, or an exclusive holder that did not let go.
type RefusedError struct {
	Room string
	// Own is this process's id, Theirs the room's: empty when the record is
	// missing or cannot be used, and Record then says which.
	Own, Theirs string
	Record      error
	// Holders hold the lease a launch asked for exclusive.
	Holders []Holder
	// Busy says the shared lease was not taken within its bound.
	Busy bool
}

// way is how past a refusal: the room's build stays while anything holds it.
const way = "end the room's sessions, or launch into another room with --room"

func (e *RefusedError) Error() string {
	room := filepath.Base(e.Room)
	switch {
	case e.Busy:
		return fmt.Sprintf("Rewake: a launch is changing the build of room %s and has not finished within %s, so nothing was changed; run it again.", room, shareWait)
	case len(e.Holders) > 0:
		names := make([]string, 0, len(e.Holders))
		for _, holder := range e.Holders {
			names = append(names, holder.String())
		}
		return fmt.Sprintf("Rewake: room %s is held by build %s and this is build %s, so nothing was changed; it is held by %s. To launch this build, %s.", room, e.theirs(), Short(e.Own), strings.Join(names, ", "), way)
	case errors.Is(e.Record, state.ErrNoBuildRecord):
		return fmt.Sprintf("Rewake: room %s has no build yet — no session was launched in it — so nothing was changed; launch a session there first.", room)
	case e.Record != nil:
		return fmt.Sprintf("Rewake: the build of room %s cannot be read (%v), and this is build %s, so nothing was changed; launching a session there sets it once nothing holds the room.", room, e.Record, Short(e.Own))
	}
	return fmt.Sprintf("Rewake: room %s belongs to build %s of rewake and this is build %s, so nothing was changed; %s.", room, Short(e.Theirs), Short(e.Own), way)
}

// theirs is the room's build as a holder diagnostic names it.
func (e *RefusedError) theirs() string {
	switch {
	case e.Theirs != "":
		return Short(e.Theirs)
	case errors.Is(e.Record, state.ErrNoBuildRecord):
		return "none"
	}
	return "unknown"
}

// Acquire takes the room's lease for a writer: shared, then the build
// checked. The writer's own id goes on; another id, or a record that is
// missing, unreadable or malformed, refuses before the first effect. Only a
// launch establishes a room's build, so a room no launch has written is no
// room this build may write, and a writer makes no room.
func Acquire(base, room string) (*Lease, error) {
	return acquire(base, room, false)
}

// AcquireLaunch takes the room's lease for a launch: as a writer, except
// that a room whose record is missing, unreadable or another build's is
// switched to this build — the lock taken exclusive without waiting, which any
// holder refuses, the record written, the lock turned shared and the record
// read again like any writer's, since flock does not promise the conversion
// is atomic.
func AcquireLaunch(base, room string) (*Lease, error) {
	return acquire(base, room, true)
}

func acquire(base, roomName string, launch bool) (*Lease, error) {
	own, err := ID()
	if err != nil {
		return nil, err
	}
	room, err := enter(base, roomName, launch)
	if err != nil {
		return nil, err
	}
	open := state.OpenLeaseLock
	if launch {
		open = state.CreateLeaseLock
	}
	lock, err := open(room)
	if err != nil {
		return nil, noBuild(room, own, err)
	}
	lease, err := hold(lock, room, own, launch)
	if err != nil {
		lock.Close()
		return nil, noBuild(room, own, err)
	}
	return lease, nil
}

// noBuild turns a room without its lock — never made, or gone while the lease
// was taken — into the refusal of a room without a build: no launch completed
// it, and a writer completes nothing.
func noBuild(room, own string, err error) error {
	if errors.Is(err, state.ErrNoLeaseLock) {
		return &RefusedError{Room: room, Own: own, Record: state.ErrNoBuildRecord}
	}
	return err
}

// enter gives the room a lease is taken in. A launch makes what is missing
// of it; a writer makes nothing — a room no launch has completed has no build
// it could write under, and its refusal leaves the base as it found it.
func enter(base, roomName string, launch bool) (string, error) {
	if launch {
		room, err := state.Bootstrap(base, roomName)
		if err != nil {
			return "", &UnusableError{Err: err}
		}
		return room, nil
	}
	room, err := state.RoomPath(base, roomName)
	if err != nil {
		return "", &UnusableError{Err: err}
	}
	if _, err := os.Lstat(room); errors.Is(err, os.ErrNotExist) {
		return "", &RefusedError{Room: room, Record: state.ErrNoBuildRecord}
	}
	if err := state.Verify(room); err != nil {
		return "", &UnusableError{Err: err}
	}
	return room, nil
}

func hold(lock *state.LeaseLock, room, own string, launch bool) (*Lease, error) {
	if err := share(lock, room, own); err != nil {
		return nil, err
	}
	theirs, err := state.ReadBuildRecord(room)
	switch {
	case err == nil && theirs == own:
		return grant(lock, room)
	case launch:
		return switchBuild(lock, room, own)
	}
	return nil, &RefusedError{Room: room, Own: own, Theirs: theirs, Record: err}
}

// switchWait bounds a launch's attempts to switch the room's build (D4a): a
// holder that is another launch of this build switching the same room lets go
// within it, and a holder still there after it — a session of another build —
// is refused.
var switchWait = 2 * time.Second

// switchBuild makes this launch's id the room's build, under the exclusive
// lock, and holds the lease shared afterwards. The exclusive request never
// waits for a holder, but a refused request is not yet a refusal: two launches
// of one build into a room without its record both read it missing, and
// flock's conversion lets only one of them through. The other takes the lock
// shared again, within the writers' bound, and reads the record: its own build
// means the winner switched it, and the room is granted. Anything else is
// tried again from an unlocked lock after a short random pause, until
// switchWait, and refused then, naming the holders.
func switchBuild(lock *state.LeaseLock, room, own string) (*Lease, error) {
	state.Step("lease switch")
	deadline := time.Now().Add(switchWait)
	for {
		taken, err := lock.TryExclusive()
		if err != nil {
			return nil, err
		}
		if taken {
			return switched(lock, room, own)
		}
		if err := share(lock, room, own); err != nil {
			return nil, err
		}
		theirs, record := state.ReadBuildRecord(room)
		if record == nil && theirs == own {
			return grant(lock, room)
		}
		if !time.Now().Before(deadline) {
			return nil, &RefusedError{Room: room, Own: own, Theirs: theirs, Record: record, Holders: holdersOf(lock)}
		}
		if err := lock.Unlock(); err != nil {
			return nil, err
		}
		time.Sleep(switchPause + rand.N(switchPause))
	}
}

// switchPause is the least pause between a launch's attempts to switch; each
// adds a random part as long, so two launches retrying together drift apart.
const switchPause = 10 * time.Millisecond

// switched writes this launch's id under the exclusive lock it took and turns
// the lock shared.
func switched(lock *state.LeaseLock, room, own string) (*Lease, error) {
	state.Step("lease exclusive")
	// Read again under the exclusive lock: a launch of this same build may
	// have written it meanwhile.
	if current, err := state.ReadBuildRecord(room); err != nil || current != own {
		if err := state.WriteBuildRecord(lock, own); err != nil {
			return nil, err
		}
	}
	state.Step("lease downgrade")
	if err := share(lock, room, own); err != nil {
		return nil, err
	}
	// Whoever took the lock exclusive between the conversion's release and its
	// shared lock may have written another build.
	if after, err := state.ReadBuildRecord(room); err != nil || after != own {
		return nil, &RefusedError{Room: room, Own: own, Theirs: after, Record: err}
	}
	return grant(lock, room)
}

func share(lock *state.LeaseLock, room, own string) error {
	err := lock.Share(shareWait)
	if errors.Is(err, state.ErrLeaseBusy) {
		return &RefusedError{Room: room, Own: own, Busy: true}
	}
	return err
}

func grant(lock *state.LeaseLock, room string) (*Lease, error) {
	if err := state.Authorize(lock); err != nil {
		return nil, err
	}
	state.Step("leased")
	return &Lease{lock: lock, room: room, holds: 1}, nil
}
