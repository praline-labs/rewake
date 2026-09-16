package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// errNotASession means this process was not started by a rewake wrapper.
var errNotASession = errors.New("this shell is not part of a rewake session")

// errEarlierRun means the name now belongs to a later run than this process.
var errEarlierRun = errors.New("this shell belongs to a session that has ended; its name is used by another one now")

// ownRun returns the session this process belongs to and the run of it.
//
// The name comes from the environment, and so does the run. A name outlives its
// session, and a process left behind by one run — a background job, a stray
// shell — would otherwise read and answer for the next. A wrapper from before
// runs were passed down sets no epoch; its record's epoch is taken then.
func ownRun(dir string) (registry.Session, string, error) {
	name := os.Getenv(state.SessionEnv)
	if name == "" {
		return registry.Session{}, "", errNotASession
	}
	session, err := registry.Lookup(dir, name)
	if errors.Is(err, registry.ErrNotFound) {
		return registry.Session{}, "", fmt.Errorf("the session %s is not registered any more", name)
	}
	if err != nil {
		return registry.Session{}, "", fmt.Errorf("could not read the record of %s: %w", name, err)
	}
	epoch := os.Getenv(state.EpochEnv)
	if epoch == "" {
		return session, session.Epoch(), nil
	}
	if epoch != session.Epoch() {
		return session, epoch, errEarlierRun
	}
	return session, epoch, nil
}
