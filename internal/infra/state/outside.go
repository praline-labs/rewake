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

// admission is one folder admitted for a room, for the length of one call.
type admission struct{ room, folder string }

var admitted struct {
	mu   sync.Mutex
	held map[admission]int
}

// WithOutside runs fn with writes under folder admitted for room, as under a
// leased room: each write inside asks the room's lease again, so one made
// after the lease went is refused, and reaches no link below the folder. It
// refuses unless the process holds the room's lease, and folder is absolute,
// clean, reaches no link and lies apart from the state base: a folder that
// held the base would admit every room in it.
func WithOutside(room, folder string, fn func() error) error {
	if !leasedRoom(filepath.Clean(room)) {
		return refuseNoLease(opLock, room)
	}
	if err := checkOutside(room, folder); err != nil {
		return err
	}
	entry := admission{room: filepath.Clean(room), folder: folder}
	admitted.mu.Lock()
	if admitted.held == nil {
		admitted.held = map[admission]int{}
	}
	admitted.held[entry]++
	admitted.mu.Unlock()
	defer func() {
		admitted.mu.Lock()
		defer admitted.mu.Unlock()
		if admitted.held[entry]--; admitted.held[entry] <= 0 {
			delete(admitted.held, entry)
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
// admits now, for a room whose lease is still held, with no link on the way
// from the folder to it: a link put where a directory was would take the
// write elsewhere. The walk is a check at the write, not a lock on the tree.
func admittedOutside(path string) bool {
	admitted.mu.Lock()
	defer admitted.mu.Unlock()
	for entry := range admitted.held {
		if within(path, entry.folder) && leasedRoom(entry.room) && linkFree(entry.folder, path) {
			return true
		}
	}
	return false
}

// linkFree says that no part of path below folder, as far as it exists, is a
// link.
func linkFree(folder, path string) bool {
	rest, err := filepath.Rel(folder, path)
	if err != nil {
		return false
	}
	at := folder
	for _, part := range strings.Split(rest, string(filepath.Separator)) {
		if part == "." {
			continue
		}
		at = filepath.Join(at, part)
		info, err := os.Lstat(at)
		if err != nil {
			return true // not made yet, nothing to follow
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	return true
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
