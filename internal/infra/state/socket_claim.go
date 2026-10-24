package state

import (
	"fmt"
	"os"
	"path/filepath"
)

// SocketClaim is a run's authority over one socket path it owns outside the
// state (docs/rules/state.md#what-is-a-writer): the reply socket a
// harness's receipt protocol puts beside a socket the caller named, wherever
// that is. It covers that one path and three acts on it — clearing a dead
// run's socket, binding, and the close-time unlink — and only while this
// process holds the room's lease, behind view mode as every write is. It
// widens nothing of any state: a path in a state directory, the room's own
// base or another, is guarded as any other write there, by the lease of the
// room it lies in, as written and where its links lead.
type SocketClaim struct {
	room, path string
}

// ClaimSocket claims path for the run of room. The room has to be a room of
// a state directory and the path absolute and clean; whether the lease is held, and
// where the path leads, is asked at each act, not here, so a claim kept past
// the lease writes nothing.
func ClaimSocket(room, path string) (*SocketClaim, error) {
	if _, err := RootForRoom(room); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("the socket path %s is not absolute", path)
	}
	if path != filepath.Clean(path) {
		return nil, fmt.Errorf("the socket path %s is not clean", path)
	}
	return &SocketClaim{room: room, path: path}, nil
}

// check takes the claimed path spelled exactly as claimed, never one that
// only cleans to it: the kernel follows a link before a "..", so the spelling
// an act passes on is the one checked, and a clean one is the only spelling
// whose lexical guard and kernel walk agree.
func (c *SocketClaim) check(op, path string) error {
	if path != c.path {
		return fmt.Errorf("%s is not the socket %s claimed", path, c.path)
	}
	if seen := c.seen(); inState(seen) {
		return guard(op, seen...)
	}
	if err := viewWrite(op, path); err != nil {
		return err
	}
	if !leased(c.room) {
		return refuseNoLease(op, path)
	}
	return nil
}

// seen is the claimed path as written and where its directory's links lead,
// which is where a bind lands. Both are guarded once either lies in a state:
// the written one keeps a link in another room that room's write, the
// resolved one keeps a link in the leased room from lending its lease to
// where it leads. What is decided is by what is on disk at the act; a link
// swapped between the check and the act is not seen, as for every guarded
// write.
func (c *SocketClaim) seen() []string {
	seen := []string{c.path}
	if dir, err := filepath.EvalSymlinks(filepath.Dir(c.path)); err == nil {
		if resolved := filepath.Join(dir, filepath.Base(c.path)); resolved != c.path {
			seen = append(seen, resolved)
		}
	}
	return seen
}

// inState says whether any of paths lies in a state directory. Outside its
// own base is no proof: another base is a state too, and so is a room of 1.x.
func inState(paths []string) bool {
	for _, path := range paths {
		for dir := path; ; dir = filepath.Dir(dir) {
			if stateDir(dir) {
				return true
			}
			if filepath.Dir(dir) == dir {
				break
			}
		}
	}
	return false
}

// stateDir says whether dir is a base, which holds a root with rooms, so
// whatever lies below it is a state's, or a room by what it holds, of either
// layout, which finds a room of 1.x where no root is. A directory that only
// looks like one is taken for one too, the safe side.
func stateDir(dir string) bool {
	return isDir(dir, rootName, "rooms") ||
		isDir(dir, sessionsDir) && isDir(dir, inboxDir) && isDir(dir, socketsDir)
}

func isDir(parts ...string) bool {
	info, err := os.Lstat(filepath.Join(parts...))
	return err == nil && info.IsDir()
}

// Listen binds the claimed path as the state's Listen binds a room's: its
// close removes the path once, under the same claim.
func (c *SocketClaim) Listen(path string) (*Listener, error) {
	return listen(path, func(op string) error { return c.check(op, path) })
}

// Discard removes a socket left at the claimed path, a dead run's; anything
// else there is not the run's to remove.
func (c *SocketClaim) Discard(path string) error {
	if err := c.check(OpRemove, path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("%s is not a socket", path)
	}
	return os.Remove(path)
}
