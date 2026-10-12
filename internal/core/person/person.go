// Package person tells whether a process may decide as the person: run
// positively outside every session of the selected v2 root
// (docs/v2/stage3-decision.md#authority, Q3 of
// docs/v2/stage3-decision-build.md). Failing to find a session is no proof of
// being outside one, so every read the answer stands on must succeed; one that
// does not refuses, as does a scan that does not hold still.
//
// The accepted limit, unverified by design: a process that left a session's
// tree through setsid and a double fork, with a clean environment, has nothing
// left in its chain to tell it apart from the person's own.
package person

import (
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// UnreadableError is an identity that could not be read whole: exit 1. It is
// never taken for the person.
type UnreadableError struct {
	What string
	Err  error
}

func (e *UnreadableError) Error() string {
	return fmt.Sprintf("whether this process runs inside a session cannot be told, so it may not decide as the person: %s: %v", e.What, e.Err)
}

func (e *UnreadableError) Unwrap() error { return e.Err }

// BelowError is a process found positively inside a session: exit 2.
type BelowError struct {
	// PID is the ancestor that places the caller inside: a wrapper, or a
	// process started with the session's variable.
	PID int
	// Session and Room name the session when a wrapper was matched.
	Session, Room string
}

func (e *BelowError) Error() string {
	if e.Session != "" {
		return fmt.Sprintf("this process runs below the wrapper of session %s in room %s (pid %d), so it does not decide as the person", e.Session, e.Room, e.PID)
	}
	return fmt.Sprintf("process %d above this one was started inside a session (%s in its environment), so this process does not decide as the person", e.PID, sessionEnv)
}

// sessionEnv is the variable a wrapper gives its harness. It is read from
// each ancestor's initial environment and never trusted for who the session
// is: only its presence places a process inside one.
const sessionEnv = "REWAKE_SESSION"

// attempts bounds the scans: a chain or registry that keeps changing under
// the scan is refused rather than waited out.
const attempts = 3

// Scan is one classification: the processes as a /proc tree shows them and
// the registry of every room of a root.
type Scan struct {
	Proc proc.Reader
	// Root is the selected v2 root.
	Root string
	// Self is the caller, and UID its user.
	Self, UID int
	// inventory reads one room strictly; registry.Inventory unless a test
	// stands in for it.
	inventory func(room string) ([]registry.Inventoried, error)
}

// Classify answers nil for the person, a *BelowError for a process inside a
// session and a *UnreadableError when that cannot be told.
func Classify(root string) error {
	return Scan{Proc: proc.Default, Root: root, Self: os.Getpid(), UID: os.Getuid()}.Classify()
}

// errUnstable is a scan whose reads disagreed: it starts again.
var errUnstable = errors.New("the processes changed while they were read")

// Classify runs the scan until two readings of the chain agree around one
// reading of every room, up to its bound.
func (s Scan) Classify() error {
	if s.inventory == nil {
		s.inventory = registry.Inventory
	}
	var last error
	for range attempts {
		err := s.once()
		if !errors.Is(err, errUnstable) {
			return err
		}
		last = err
	}
	return &UnreadableError{What: fmt.Sprintf("the scan did not hold still in %d attempts", attempts), Err: last}
}

func (s Scan) once() error {
	first, err := s.chain()
	if err != nil {
		return err
	}
	sessions, err := s.sessions()
	if err != nil {
		return err
	}
	second, err := s.chain()
	if err != nil {
		return err
	}
	if !slices.Equal(first, second) {
		return fmt.Errorf("%w: the chain read %v, then %v", errUnstable, first, second)
	}
	return judge(first, sessions)
}

// roomRecord is one session record with the room it was found in.
type roomRecord struct {
	room string
	registry.Inventoried
}

// sessions reads every room of the root strictly. It takes no lock or lease
// in any room and repairs nothing: a reader of other rooms must not change
// them.
func (s Scan) sessions() ([]roomRecord, error) {
	rooms, err := state.AllRoomDirs(s.Root)
	if err != nil {
		return nil, &UnreadableError{What: "the rooms of " + s.Root, Err: err}
	}
	var found []roomRecord
	for _, room := range rooms {
		records, err := s.inventory(room)
		if err != nil {
			return nil, &UnreadableError{What: "the sessions of room " + room, Err: err}
		}
		for _, record := range records {
			found = append(found, roomRecord{room: room, Inventoried: record})
		}
	}
	return found, nil
}

// judge places the chain against the sessions: a session variable anywhere in
// it, or a live wrapper that is one of its processes, is inside; a wrapper
// whose run cannot be judged leaves the answer unknown.
func judge(chain []link, sessions []roomRecord) error {
	for _, l := range chain {
		if l.session {
			return &BelowError{PID: l.pid}
		}
	}
	for _, record := range sessions {
		session := record.Session
		switch record.Run {
		case proc.IdentityUnknown:
			return &UnreadableError{
				What: fmt.Sprintf("whether the wrapper of session %s in room %s runs", session.Name, record.room),
				Err:  errors.New("its process cannot be judged from here"),
			}
		case proc.IdentityAlive:
			for _, l := range chain {
				if l.pid != session.ServicePID {
					continue
				}
				if l.start != session.ServiceStart {
					// The wrapper was seen alive under the start its record
					// names, and this pid under another both before and after.
					return fmt.Errorf("%w: pid %d is the wrapper of %s and not", errUnstable, l.pid, session.Name)
				}
				return &BelowError{PID: l.pid, Session: session.Name, Room: record.room}
			}
		}
	}
	return nil
}
