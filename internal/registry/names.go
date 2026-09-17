package registry

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

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
		if !state.ValidName(candidate) {
			return "", fmt.Errorf("%w: automatic name %q must fit 32 characters and the session-name syntax; choose a shorter valid prefix with --name", ErrUnusableName, candidate)
		}
		if _, err := Lookup(dir, candidate); errors.Is(err, ErrNotFound) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("every name from %s to %s-99 is in use", base, base)
}

// SocketFor is where the wrapper asks a harness to place its inbox socket.
func SocketFor(dir, name, epoch string) string { return state.SocketPath(dir, name, epoch) }

// RecordPath is the file holding a session record.
func RecordPath(dir, name string) string { return filepath.Clean(state.SessionPath(dir, name)) }
