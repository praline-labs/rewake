/*
Package registry keeps the record of every session running under rewake.

A record is one small JSON file named after the session, and the name is the
address other sessions use. Publishing is exclusive: the record is written to a
temporary file and linked into place, so two wrappers starting at once cannot
both believe they own a name.

Liveness is the pair "pid plus process start time". A pid alone lies as soon as
the number is reused, and it is reused within a day on a busy machine.
*/
package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// Session is what one running harness looks like to everybody else.
type Session struct {
	// MessagingReadyAt marks successful initial service readiness, not current connectivity.
	MessagingReadyAt *time.Time `json:"messagingReadyAt,omitempty"`
	// Name is the address of this session.
	Name string `json:"name"`
	// Room scopes the name and all mailbox paths.
	Room string `json:"room"`
	// Harness is the id of the harness being run, such as "claude".
	Harness string `json:"harness"`
	// ServicePID is the rewake process that serves this session's inbox, and
	// whose life defines the life of the session.
	ServicePID int `json:"servicePid"`
	// ServiceStart is the start time of that process, in clock ticks.
	ServiceStart uint64 `json:"serviceStart"`
	// HarnessPID is the process of the harness itself; zero until it starts.
	HarnessPID int `json:"harnessPid,omitempty"`
	// HarnessStart is the start time of the harness process.
	HarnessStart uint64 `json:"harnessStart,omitempty"`
	// CWD is where the harness was started.
	CWD string `json:"cwd"`
	// StartedAt is when the session was published.
	StartedAt time.Time `json:"startedAt"`
	// Socket is the inbox socket of harnesses that have one.
	Socket string `json:"socket,omitempty"`
	// OwnsSocket says this session created the socket path, and so may remove
	// it when it ends.
	OwnsSocket bool `json:"ownsSocket,omitempty"`
	// CodexHome is the CODEX_HOME the session runs with, for harnesses that
	// keep their state there.
	CodexHome string `json:"codexHome,omitempty"`
	// Role is what the session is for, an id from package role. Empty means
	// the default role.
	Role string `json:"role"`
	// RoleReason records why this launch received its role.
	RoleReason string `json:"roleReason,omitempty"`
	// PIDNamespace is the pid namespace the two pids above belong to. A reader
	// in a different one cannot judge whether they are alive.
	PIDNamespace string `json:"pidNamespace,omitempty"`
	// Boot and Build say which boot of the machine the run belongs to and
	// which protocol it follows (run.go); an earlier build wrote neither.
	Boot  string `json:"boot,omitempty"`
	Build string `json:"build,omitempty"`
	// AssumedGates are the gates this launch took as closed through the
	// verification switch of the live checks
	// (docs/mail-bridge-launch.md#gates-taken-as-closed), so whoami and
	// list show a run whose tool rests on an assumption.
	AssumedGates []string `json:"assumedGates,omitempty"`
}

// ErrNotFound is returned when no session answers to a name.
var ErrNotFound = errors.New("no such session")

// ErrUnusableName marks a name that cannot address a session, which is a wrong
// call rather than a failure of the target.
var ErrUnusableName = errors.New("unusable session name")

// Publish claims a name for a session.
//
// Every change to a name happens under that name's lock — claiming it, taking it
// over from a session that has ended, removing it. A lock around only part of
// that was no lock at all: a reader pruning the old record, or a second claimant
// on the free path, still slipped between the steps of a takeover and the two
// ended up serving one mailbox.
func Publish(dir string, session Session) error {
	if !state.ValidName(session.Name) {
		return fmt.Errorf("%w: %q. Use lower-case letters, digits, dot, dash or underscore, up to 32 characters", ErrUnusableName, session.Name)
	}
	encoded, err := encode(session)
	if err != nil {
		return err
	}
	path := state.SessionPath(dir, session.Name)

	return state.WithNameLock(dir, session.Name, func() error {
		existing, loadErr := Load(dir, session.Name)
		switch {
		case loadErr == nil && existing.Alive():
			return &NameTakenError{Name: session.Name, PID: existing.ServicePID}
		case loadErr == nil, errors.Is(loadErr, ErrNotFound):
			// Either free, a leftover of a session that ended, or a record that
			// cannot be read at all. All three are ours to replace: nothing else
			// may touch this name while the lock is held.
		default:
			return loadErr
		}

		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return removeErr
		}
		if err := state.PublishExclusive(path, encoded); err != nil {
			if errors.Is(err, state.ErrNameTaken) {
				return &NameTakenError{Name: session.Name}
			}
			return err
		}
		return nil
	})
}

// NameTakenError says a session name belongs to somebody else. It is a distinct
// type because the caller can act on it: an automatic name simply tries the next
// one, while an explicit name is a wrong call and has to be reported as such.
type NameTakenError struct {
	Name string
	PID  int
}

func (e *NameTakenError) Error() string {
	if e.PID != 0 {
		return fmt.Sprintf("the name %q is taken by a live session (pid %d)", e.Name, e.PID)
	}
	return fmt.Sprintf("the name %q was claimed by another session while starting", e.Name)
}

// Update rewrites the record of a session that still owns its name.
//
// Under the lock and behind an ownership check, like every other change to a
// name: an update that was slow to arrive would otherwise land after the name
// changed hands and replace the new owner's record with a stale one.
func Update(dir string, session Session) error {
	if !state.ValidName(session.Name) {
		return fmt.Errorf("%w: %q", ErrUnusableName, session.Name)
	}
	encoded, err := encode(session)
	if err != nil {
		return err
	}
	return state.WithNameLock(dir, session.Name, func() error {
		existing, err := Load(dir, session.Name)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return ErrNotFound
			}
			return err
		}
		if existing.Epoch() != session.Epoch() {
			return ErrNotFound
		}
		return state.WriteAtomic(state.SessionPath(dir, session.Name), encoded)
	})
}

// Remove deletes the record of a name, whoever owns it. Use RemoveOwned unless
// the caller really means "this name, whatever is behind it".
func Remove(dir, name string) error {
	if !state.ValidName(name) {
		return nil
	}
	return state.WithNameLock(dir, name, func() error {
		err := os.Remove(state.SessionPath(dir, name))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	})
}

// OwnsName reports whether the record of this name still describes the session
// with this epoch.
func OwnsName(dir, name, epoch string) bool {
	existing, err := Load(dir, name)
	return err == nil && existing.Epoch() == epoch
}

// RemoveOwned deletes a record only while it still describes this session, and
// reports whether it did.
//
// A wrapper that is stopped, whose harness then exits, wakes up to a name that
// may already belong to somebody else. Removing it by name would delete a live
// session's record — seen happening — so the epoch decides. The answer comes
// back rather than being asked for separately: between a question and an action
// the name can change hands, and then the action lands on the wrong session.
func RemoveOwned(dir, name, epoch string) (bool, error) {
	if !state.ValidName(name) {
		return false, nil
	}
	removed := false
	err := state.WithNameLock(dir, name, removeIfOwned(dir, name, epoch, &removed))
	return removed, err
}

// pruneIfFree removes a dead run's record the way RemoveOwned does, but only
// when nobody holds the name: listing and lookup are reads, and a read must
// not wait on a lock indefinitely. Whoever holds it is the run leaving or another caller
// cleaning up, and a record left now is pruned by the next listing.
func pruneIfFree(dir, name, epoch string) {
	if !state.ValidName(name) {
		return
	}
	removed := false
	_, _ = state.TryWithNameLock(dir, name, removeIfOwned(dir, name, epoch, &removed))
}

func removeIfOwned(dir, name, epoch string, removed *bool) func() error {
	return func() error {
		existing, err := Load(dir, name)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil
			}
			return err
		}
		if existing.Epoch() != epoch {
			return nil
		}
		err = os.Remove(state.SessionPath(dir, name))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err == nil {
			*removed = true
		}
		return err
	}
}

// Load reads one record without judging whether it is alive.
func Load(dir, name string) (Session, error) {
	// A name becomes a file path, so an unusable one must never reach the file
	// system: "../../victim" would otherwise read — and, through Lookup, delete
	// — a file outside the state directory.
	if !state.ValidName(name) {
		return Session{}, ErrNotFound
	}
	raw, err := os.ReadFile(state.SessionPath(dir, name))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Session{}, ErrNotFound
		}
		return Session{}, err
	}
	var session Session
	if err := json.Unmarshal(raw, &session); err != nil {
		return Session{}, fmt.Errorf("the record of session %q is unreadable: %w", name, err)
	}
	if session.ServicePID <= 0 || session.ServiceStart == 0 {
		return Session{}, fmt.Errorf("the record of session %q has no process identity", name)
	}
	return session, nil
}

// Lookup returns a live session by name.
func Lookup(dir, name string) (Session, error) {
	session, err := Load(dir, name)
	if err != nil {
		return Session{}, err
	}
	if !session.Alive() {
		if session.Judgeable() {
			// Reading is also when leftovers are cleaned: nobody else will. It
			// is the record that was read that goes, not whatever holds the
			// name by the time the lock is taken — and only when the lock is
			// free: turn-ended looks sessions up from a foreground Stop hook,
			// and a lookup that waited on a held name would stall the end of a
			// turn. The answer does not depend on it: a dead record is not a
			// live session whether or not it is gone yet.
			pruneIfFree(dir, name, session.Epoch())
		}
		return Session{}, ErrNotFound
	}
	return session, nil
}

// List returns every live session, oldest first, and removes the records of
// sessions that have ended.
func List(dir string) ([]Session, error) { return list(dir, true) }

// ListReadOnly observes live records without acquiring cleanup/name locks.
func ListReadOnly(dir string) ([]Session, error) { return list(dir, false) }

func list(dir string, cleanup bool) ([]Session, error) {
	entries, err := os.ReadDir(state.SessionsPath(dir))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var sessions []Session
	for _, entry := range entries {
		name := strings.TrimSuffix(entry.Name(), ".json")
		if entry.IsDir() || name == entry.Name() || strings.HasPrefix(name, ".") {
			continue
		}
		session, err := Load(dir, name)
		if err != nil {
			continue
		}
		if !session.Alive() {
			if cleanup && session.Judgeable() {
				pruneIfFree(dir, name, session.Epoch())
			}
			continue
		}
		sessions = append(sessions, session)
	}

	sortByStart(sessions)
	return sessions, nil
}

func encode(session Session) ([]byte, error) {
	encoded, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func sortByStart(sessions []Session) {
	for i := 1; i < len(sessions); i++ {
		for j := i; j > 0 && sessions[j].StartedAt.Before(sessions[j-1].StartedAt); j-- {
			sessions[j], sessions[j-1] = sessions[j-1], sessions[j]
		}
	}
}
