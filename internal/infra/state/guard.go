package state

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

// The guard of the write seam (docs/rules/state.md#what-is-a-writer):
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
			return refuseNoLease(op, path)
		}
	}
	return nil
}

// refuseNoLease counts a refusal for want of a lease and gives its error.
func refuseNoLease(op, path string) error {
	refusedNoLease.Add(1)
	return &os.PathError{Op: op, Path: path, Err: ErrNoLease}
}

// refusedNoLease counts the writes refused for want of a lease, the
// refusals a caller swallowed among them.
var refusedNoLease atomic.Int64

// NoLeaseRefusals is how many writes this process has been refused for want
// of a lease. Code that meets a refusal may swallow it, as a cleanup does;
// the count still has it, so a test can prove an invocation made no write
// outside its lease, its last effect included.
func NoLeaseRefusals() int64 { return refusedNoLease.Load() }

// leased answers whether path is a room this process holds a lease for, or
// lies inside one. The comparison is by cleaned path, as the lease names its
// room: the resolvers that give a write its path give the lease its room.
func leased(path string) bool {
	clean := filepath.Clean(path)
	return leasedRoom(clean) || admittedOutside(clean)
}

// leasedRoom is leased for a path in a room only.
func leasedRoom(clean string) bool {
	leases.mu.Lock()
	defer leases.mu.Unlock()
	for room := range leases.held {
		if clean == room || strings.HasPrefix(clean, room+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
