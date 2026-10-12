package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/praline-labs/rewake/internal/core/role"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// errNotASession means this process was not started by a rewake wrapper.
var errNotASession = errors.New("this shell is not part of a rewake session")

// errNoRun means the process has a session name but no run: its wrapper came
// from a version that did not pass one down.
var errNoRun = errors.New("this shell was started by an older rewake that did not record which run it belongs to; restart the session")

// errEarlierRun means the name now belongs to a later run than this process.
var errEarlierRun = errors.New("this shell belongs to a session that has ended; its name is used by another one now")

// ownRun returns the session this process belongs to and the run of it.
//
// The name comes from the environment, and so does the run. A name outlives its
// session, and a process left behind by one run — a background job, a stray
// shell — would otherwise read and answer for the next. A process without a run
// is refused rather than given the current one: by its name alone, a leftover
// from an ended session cannot be told from the session that holds it now.
func ownRun(dir string) (registry.Session, string, error) { return ownRunBy(dir, registry.Lookup) }

// ownRunIn is ownRun for a form of this class: a view looks the caller up
// without pruning a dead record, which is a write
// (docs/v2/stage3-state.md#what-is-a-writer).
func ownRunIn(class access, dir string) (registry.Session, string, error) {
	if class == accessView {
		return ownRunBy(dir, registry.LookupReadOnly)
	}
	return ownRun(dir)
}

func ownRunBy(dir string, lookup func(dir, name string) (registry.Session, error)) (registry.Session, string, error) {
	name := os.Getenv(state.SessionEnv)
	if name == "" {
		return registry.Session{}, "", errNotASession
	}
	session, err := lookup(dir, name)
	if errors.Is(err, registry.ErrNotFound) {
		return registry.Session{}, "", fmt.Errorf("the session %s is not registered any more", name)
	}
	if err != nil {
		return registry.Session{}, "", fmt.Errorf("could not read the record of %s: %w", name, err)
	}
	epoch := os.Getenv(state.EpochEnv)
	if epoch == "" {
		return session, "", errNoRun
	}
	if epoch != session.Epoch() {
		return session, epoch, errEarlierRun
	}
	return session, epoch, nil
}

// callerPlaybook is the role guidance for the session running this command, or
// nil when the command was not run by one. Only views ask for it — the guide
// and help pages — so it looks the caller up without pruning.
//
// A guide asked for outside a session stays the general map: there is no role
// to answer for, and inventing one would teach the wrong moves to whoever is
// reading over a person's shoulder. Every failure answers the same way — an
// unreadable state directory, a name that is not registered any more, a run
// that has moved on — because in each of those the role is not known, which is
// the only question being asked here.
func callerPlaybook() *role.Playbook {
	// The room every other command resolves: REWAKE_ROOM, the default room
	// without it.
	dir, err := state.Dir()
	if err != nil {
		return nil
	}
	session, _, err := ownRunIn(accessView, dir)
	if err != nil {
		return nil
	}
	play := role.Of(session.Role).Play
	return &play
}
