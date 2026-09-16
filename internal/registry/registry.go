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
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/proc"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// Session is what one running harness looks like to everybody else.
type Session struct {
	// Name is the address of this session.
	Name string `json:"name"`
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
	// PIDNamespace is the pid namespace the two pids above belong to. A reader
	// in a different one cannot judge whether they are alive.
	PIDNamespace string `json:"pidNamespace,omitempty"`
}

// alive is the liveness check, replaceable so tests can describe a machine
// instead of running processes on the real one.
var alive = proc.Alive

// namespace reports the pid namespace of this process, replaceable in tests.
var namespace = proc.Namespace

// Reachable reports whether a session may still be sent to. It is not the same
// question as Alive: a reader that cannot see the processes says yes, because
// the wrapper that can see them is the one that will deliver.
func (s Session) Reachable() bool { return s.Alive() }

// Judgeable reports whether this reader can tell if the session is running.
//
// Only a reader in the same pid namespace can. A Codex agent runs its commands
// in a sandbox with its own namespace, where every pid but its own is missing —
// and a reader that mistook that for death reported live sessions as gone and
// deleted their records. Found by running it.
func (s Session) Judgeable() bool {
	here := namespace()
	if here == "" {
		// This reader cannot even tell which namespace it is in, so it cannot
		// tell whether a pid means anything here. Guessing the other way
		// deleted the record of a session that was running.
		return false
	}
	// A record without a namespace comes from a version that did not record
	// one; the reader's own is the best it has.
	return s.PIDNamespace == "" || s.PIDNamespace == here
}

// Alive reports whether this session can still be reached: both the process
// serving the mailbox and the harness itself have to be running. A wrapper that
// outlives its harness has nothing to deliver to, and a harness whose wrapper is
// gone has nobody to deliver for it.
func (s Session) Alive() bool {
	if !s.Judgeable() {
		// Cannot see those processes from here. Saying "alive" leaves delivery
		// to the wrapper, which can see them; saying "dead" would drop mail and
		// remove a record belonging to a session that is running.
		return true
	}
	if !alive(s.ServicePID, s.ServiceStart) {
		return false
	}
	if s.HarnessPID != 0 && !alive(s.HarnessPID, s.HarnessStart) {
		return false
	}
	return true
}

// Epoch identifies this run of a session name. A message carries the epoch of
// the session it was written for, so a later session that happens to take the
// same name does not receive somebody else's mail.
func (s Session) Epoch() string {
	return strconv.Itoa(s.ServicePID) + "." + strconv.FormatUint(s.ServiceStart, 10)
}

// Age is how long the session has been published.
func (s Session) Age() time.Duration { return time.Since(s.StartedAt) }

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

// RemoveOwned deletes a record only while it still describes this session.
//
// A wrapper that is stopped, whose harness then exits, wakes up to a name that
// may already belong to somebody else. Removing it by name would delete a live
// session's record — seen happening — so the epoch decides.
func RemoveOwned(dir, name, epoch string) error {
	if !state.ValidName(name) {
		return nil
	}
	return state.WithNameLock(dir, name, func() error {
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
		return err
	})
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
			// name by the time the lock is taken.
			_ = RemoveOwned(dir, name, session.Epoch())
		}
		return Session{}, ErrNotFound
	}
	return session, nil
}

// List returns every live session, oldest first, and removes the records of
// sessions that have ended.
func List(dir string) ([]Session, error) {
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
			if session.Judgeable() {
				_ = RemoveOwned(dir, name, session.Epoch())
			}
			continue
		}
		sessions = append(sessions, session)
	}

	sortByStart(sessions)
	return sessions, nil
}

// Names lists the names of live sessions, for refusals that should say who is
// actually reachable.
func Names(dir string) []string {
	sessions, err := List(dir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(sessions))
	for _, session := range sessions {
		names = append(names, session.Name)
	}
	return names
}

// ChooseName returns a free name. An explicit one is taken as given — a taken
// name is a refusal, not a silent rename, because the caller is about to tell
// somebody else that address. A default one grows a suffix until it is free.
func ChooseName(dir, explicit, base string) (string, error) {
	if explicit != "" {
		if !state.ValidName(explicit) {
			return "", fmt.Errorf("%w: %q. Use lower-case letters, digits, dot, dash or underscore, up to 32 characters", ErrUnusableName, explicit)
		}
		return explicit, nil
	}
	for attempt := 1; attempt < 100; attempt++ {
		candidate := base
		if attempt > 1 {
			candidate = base + "-" + strconv.Itoa(attempt)
		}
		if _, err := Lookup(dir, candidate); errors.Is(err, ErrNotFound) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("every name from %s to %s-99 is in use", base, base)
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

// SocketFor is where the wrapper asks a harness to place its inbox socket.
func SocketFor(dir, name string) string { return state.SocketPath(dir, name) }

// RecordPath is the file holding a session record.
func RecordPath(dir, name string) string { return filepath.Clean(state.SessionPath(dir, name)) }
