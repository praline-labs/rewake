package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// The bootstrap of a room (docs/v2/stage3-state.md#the-bootstrap): the room's
// lease lives in the room, so making the room is a write no lease can cover.
// Bootstrap, CreateLeaseLock, OpenLeaseLock and WriteBuildRecord are the only
// writes that work without one, and build.Acquire is their one caller: a test
// over the module fails on any other reference to them, a function value
// included.

// Bootstrap makes the base, the root, the room and the room's sessions, inbox
// and sock, each when missing, and verifies each that exists without repairing
// its mode: a directory somebody else could write is refused, not chmodded. Each
// level, made or found, is synced into its parent, and a failed sync fails the
// bootstrap, so a launch never claims a name in a room a crash could take
// away. It may stop after any level; the next call completes what is missing.
func Bootstrap(base, room string) (string, error) {
	dir, err := RoomPath(base, room)
	if err != nil {
		return "", err
	}
	if err := viewWrite(OpWrite, dir); err != nil {
		return "", err
	}
	rooms := filepath.Dir(dir)
	levels := []string{base, filepath.Dir(rooms), rooms, dir}
	for _, sub := range []string{sessionsDir, inboxDir, socketsDir} {
		levels = append(levels, filepath.Join(dir, sub))
	}
	for _, level := range levels {
		if err := makeLevel(level); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// RoomPath is where a room of the base lives, checked and made nowhere.
func RoomPath(base, room string) (string, error) {
	if !filepath.IsAbs(base) {
		return "", fmt.Errorf("%s must be an absolute path, got %q", DirEnv, base)
	}
	if !ValidName(room) {
		return "", fmt.Errorf("invalid room name %q; use lower-case letters, digits, dot, dash or underscore, up to 32 characters", room)
	}
	return filepath.Join(base, rootName, "rooms", room), nil
}

// syncLevel flushes a new level into its parent; a test makes it fail.
var syncLevel = syncDir

// makeLevel makes one directory of the bootstrap, or verifies the one there,
// and flushes it into its parent either way. A level that exists is no proof
// that its name is durable: a launch that made it may have failed to flush
// it, and a concurrent one may not have flushed it yet, so the flush is this
// launch's to make or fail on, every time.
func makeLevel(path string) error {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		if err := createLevel(path); err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("could not read the state directory %s: %w", path, err)
	}
	if err := Verify(path); err != nil {
		return err
	}
	if err := syncLevel(filepath.Dir(path)); err != nil {
		return fmt.Errorf("could not flush the state directory %s: %w", filepath.Dir(path), err)
	}
	return nil
}

// createLevel makes a missing level. One another process made meanwhile is
// left for the caller to verify like any other.
func createLevel(path string) error {
	Step("bootstrap " + filepath.Base(path))
	err := os.Mkdir(path, dirMode)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("could not create the state directory %s: %w", path, err)
	}
	return fault(OpWrite, path)
}
