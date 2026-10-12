package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SocketClaim is a run's authority over one socket path it owns outside its
// room (docs/v2/stage3-state.md#what-is-a-writer): the reply socket a
// harness's receipt protocol puts beside a socket the caller named, wherever
// that is. It covers that one path and three acts on it — clearing a dead
// run's socket, binding, and the close-time unlink — and only while this
// process holds the room's lease, behind view mode as every write is. It
// widens nothing of the state: a path inside the room's base is guarded as
// any other write there, by the lease of the room it lies in.
type SocketClaim struct {
	room, path string
	// inside says the path lies in the state, where the guard decides.
	inside bool
}

// ClaimSocket claims path for the run of room. The room has to be a room of
// a state directory and the path absolute; whether the lease is held is
// asked at each act, not here, so a claim kept past the lease writes nothing.
func ClaimSocket(room, path string) (*SocketClaim, error) {
	base, err := BaseForRoom(room)
	if err != nil {
		return nil, err
	}
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return nil, fmt.Errorf("the socket path %s is not absolute", path)
	}
	inside := clean == base || strings.HasPrefix(clean, base+string(filepath.Separator))
	return &SocketClaim{room: room, path: clean, inside: inside}, nil
}

func (c *SocketClaim) check(op, path string) error {
	if filepath.Clean(path) != c.path {
		return fmt.Errorf("%s is not the socket %s claimed", path, c.path)
	}
	if c.inside {
		return guard(op, path)
	}
	if err := viewWrite(op, path); err != nil {
		return err
	}
	if !leased(c.room) {
		return refuseNoLease(op, path)
	}
	return nil
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
