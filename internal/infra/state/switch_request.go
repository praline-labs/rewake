package state

import (
	"errors"
	"os"
	"path/filepath"
)

// A deferred switch's request (docs/rules/state.md#switching-a-running-room):
// rewake restart rewake hands its switch to a leader that outlives it and
// tries again while a session is busy. The request is a file held by the
// leader's flock for as long as it lives, so one leader waits at a time, and
// one that died leaves a file no lock holds, which the next request replaces.

const switchRequestName = "switch-request"

// ErrSwitchRequested is a request refused: a leader already waits for the
// room's sessions.
var ErrSwitchRequested = errors.New("a switch already waits for the room's sessions to be idle")

// SwitchRequestPath is the room's deferred switch's request.
func SwitchRequestPath(room string) string { return filepath.Join(room, switchRequestName) }

// RequestSwitch publishes a request under the caller's lease and answers the
// file whose flock holds it: the leader inherits it, and the kernel lets go
// when the last holder ends. A request still held refuses.
func RequestSwitch(room string, data []byte) (*os.File, error) {
	path := SwitchRequestPath(room)
	var held *os.File
	err := withSwitchLock(room, func() error {
		if old, ok := TryHold(path); ok {
			_ = old.Close()
		} else if _, err := os.Stat(path); err == nil {
			return ErrSwitchRequested
		}
		var err error
		held, err = WriteAtomicHeld(path, data)
		return err
	})
	return held, err
}

// EndSwitchRequest removes the request held by held, under the caller's
// lease, and lets it go; a request another leader has since published is
// left to it.
func EndSwitchRequest(room string, held *os.File) error {
	defer func() { _ = held.Close() }()
	path := SwitchRequestPath(room)
	return withSwitchLock(room, func() error {
		named, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		opened, err := held.Stat()
		if err != nil || !os.SameFile(named, opened) {
			return nil
		}
		return Remove(path)
	})
}
