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
	// CodexHome is the CODEX_HOME the session runs with, for harnesses that
	// keep their state there.
	CodexHome string `json:"codexHome,omitempty"`
}

// alive is the liveness check, replaceable so tests can describe a machine
// instead of running processes on the real one.
var alive = proc.Alive

// Alive reports whether the process serving this session is still running.
func (s Session) Alive() bool {
	return alive(s.ServicePID, s.ServiceStart)
}

// Age is how long the session has been published.
func (s Session) Age() time.Duration { return time.Since(s.StartedAt) }

// ErrNotFound is returned when no session answers to a name.
var ErrNotFound = errors.New("no such session")

// Publish claims a name for a session. When the name is taken by a session that
// is no longer alive, the stale record is removed and the claim retried; a live
// one refuses.
func Publish(dir string, session Session) error {
	if !state.ValidName(session.Name) {
		return fmt.Errorf("%q is not a usable session name: use lower-case letters, digits, dot, dash or underscore, up to 32 characters", session.Name)
	}
	encoded, err := encode(session)
	if err != nil {
		return err
	}
	path := state.SessionPath(dir, session.Name)

	err = state.PublishExclusive(path, encoded)
	if errors.Is(err, state.ErrNameTaken) {
		existing, loadErr := Load(dir, session.Name)
		if loadErr == nil && existing.Alive() {
			return fmt.Errorf("the name %q is taken by a live session (pid %d)", session.Name, existing.ServicePID)
		}
		// The previous owner is gone: its record is a leftover, not an owner.
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return removeErr
		}
		err = state.PublishExclusive(path, encoded)
		if errors.Is(err, state.ErrNameTaken) {
			return fmt.Errorf("the name %q was claimed by another session while starting", session.Name)
		}
	}
	return err
}

// Update rewrites an existing record, for example once the harness process is
// known. The name must already belong to this session.
func Update(dir string, session Session) error {
	encoded, err := encode(session)
	if err != nil {
		return err
	}
	return state.WriteAtomic(state.SessionPath(dir, session.Name), encoded)
}

// Remove deletes a record. A record that is already gone is not an error: the
// caller wanted it gone.
func Remove(dir, name string) error {
	err := os.Remove(state.SessionPath(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Load reads one record without judging whether it is alive.
func Load(dir, name string) (Session, error) {
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
		// Reading is also when leftovers are cleaned: nobody else will.
		_ = Remove(dir, name)
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
			_ = Remove(dir, name)
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
			return "", fmt.Errorf("%q is not a usable session name: use lower-case letters, digits, dot, dash or underscore, up to 32 characters", explicit)
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
