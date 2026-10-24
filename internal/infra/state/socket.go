package state

import (
	"errors"
	"net"
	"os"
	"sync"
)

// Listener is a socket of the state, a guarded resource
// (docs/rules/state.md#what-is-a-writer): binding it makes a file in the
// room, and closing it removes that file. Go's own listener unlinks its path
// on close by itself, past any check; this one turns that off and removes the
// path itself, behind the same guard as every other write, so the close-time
// unlink is a write like the bind.
type Listener struct {
	*net.UnixListener
	path string
	// check is the guard of this socket's writes: the room's, or a
	// SocketClaim's for one outside it.
	check  func(op string) error
	remove sync.Once
}

// Listen binds a Unix socket at path, private to this user. Whatever lies at
// the path is the caller's to clear first: a dead run's socket is removed
// with Discard.
func Listen(path string) (*Listener, error) {
	return listen(path, func(op string) error { return guard(op, path) })
}

func listen(path string, check func(op string) error) (*Listener, error) {
	if err := check(OpWrite); err != nil {
		return nil, err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	listener.SetUnlinkOnClose(false)
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return &Listener{UnixListener: listener, path: path, check: check}, nil
}

// Close stops listening and removes the socket's file, once: a second close
// must not remove a socket the next run of the name has bound since. A
// removal the guard refuses leaves the file, for the next bind of the name to
// clear, and is reported; the listener is closed either way.
func (l *Listener) Close() error {
	closed := l.UnixListener.Close()
	var removed error
	l.remove.Do(func() {
		if removed = l.check(OpRemove); removed != nil {
			return
		}
		if err := os.Remove(l.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			removed = err
		}
	})
	return errors.Join(closed, removed)
}
