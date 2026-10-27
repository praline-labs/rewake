package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// A project's folder is outside the room, yet a brief may live in it
// (docs/briefs.md#where-a-brief-lives). The guard admits its writes only
// inside WithOutside, for one folder, while the room's lease is held: the
// admission ends with the function, so nothing outlasts the lock it runs
// under.

// ErrOutsideRefused is a folder that WithOutside will not admit.
var ErrOutsideRefused = errors.New("not a folder rewake may write for a room")

var admitted struct {
	mu      sync.Mutex
	folders map[string]int
}

// WithOutside runs fn with writes under folder admitted, as under a leased
// room. It refuses unless the process holds the room's lease, and folder is
// absolute, clean, reaches no link and lies apart from the state base: a
// folder that held the base would admit every room in it.
func WithOutside(room, folder string, fn func() error) error {
	if !leased(room) {
		return refuseNoLease(opLock, room)
	}
	if err := checkOutside(room, folder); err != nil {
		return err
	}
	admitted.mu.Lock()
	if admitted.folders == nil {
		admitted.folders = map[string]int{}
	}
	admitted.folders[folder]++
	admitted.mu.Unlock()
	defer func() {
		admitted.mu.Lock()
		defer admitted.mu.Unlock()
		if admitted.folders[folder]--; admitted.folders[folder] <= 0 {
			delete(admitted.folders, folder)
		}
	}()
	return fn()
}

func checkOutside(room, folder string) error {
	refuse := func(why string) error {
		return fmt.Errorf("%w: %s: %s", ErrOutsideRefused, folder, why)
	}
	if !filepath.IsAbs(folder) || filepath.Clean(folder) != folder {
		return refuse("it is not an absolute, clean path")
	}
	base, err := BaseForRoom(room)
	if err != nil {
		return err
	}
	if within(base, folder) || inState([]string{folder}) {
		return refuse("it lies in or around a rewake state directory")
	}
	// The deepest part that exists must be what its path says: a link on the
	// way would take a write somewhere the person did not name.
	existing := folder
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			break
		}
		existing = parent
	}
	if resolved, err := filepath.EvalSymlinks(existing); err != nil || resolved != existing {
		return refuse("a link lies on its path")
	}
	return nil
}

// within says whether path is dir or lies below it.
func within(path, dir string) bool {
	return path == dir || strings.HasPrefix(path, dir+string(filepath.Separator))
}

// admittedOutside answers whether path lies inside a folder WithOutside
// admits now.
func admittedOutside(path string) bool {
	admitted.mu.Lock()
	defer admitted.mu.Unlock()
	for folder := range admitted.folders {
		if within(path, folder) {
			return true
		}
	}
	return false
}

// EnsureOutsideDir creates a directory inside an admitted folder, with the
// mode a project's directories have: the state directory's own mode and
// owner checks are for rewake's rooms, not for a person's tree.
func EnsureOutsideDir(path string) error {
	if err := guard(OpWrite, path); err != nil {
		return err
	}
	return os.MkdirAll(path, 0o755)
}
