package state

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// The guard of the write seam (docs/v2/stage3-state.md#what-is-a-writer):
// one check every write primitive of a room makes before its first effect.
// A view refuses first, whatever it holds (D3); then a write of a room this
// process holds no lease for refuses. The lease is what Authorize recorded and
// a LeaseLock's Close revoked — nothing else grants it.

// ErrNoLease is the refusal of a write of a room this process holds no build
// lease for: a path in no room it leases, another room's among them.
var ErrNoLease = errors.New("this process holds no build lease for the room")

// guard refuses a write of paths in view mode, then a write of any of them
// outside the rooms this process leases. Every path a write changes is named:
// both ends of a rename or a link.
func guard(op string, paths ...string) error {
	for _, path := range paths {
		if err := viewWrite(op, path); err != nil {
			return err
		}
	}
	for _, path := range paths {
		if !leased(path) {
			return &os.PathError{Op: op, Path: path, Err: ErrNoLease}
		}
	}
	return nil
}

// leased answers whether path is a room this process holds a lease for, or
// lies inside one. The comparison is by cleaned path, as the lease names its
// room: the resolvers that give a write its path give the lease its room.
func leased(path string) bool {
	clean := filepath.Clean(path)
	leases.mu.Lock()
	defer leases.mu.Unlock()
	for room := range leases.held {
		if clean == room || strings.HasPrefix(clean, room+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
