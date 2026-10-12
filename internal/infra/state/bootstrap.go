package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// The bootstrap of a room (docs/v2/stage3-state.md#the-bootstrap): the room's
// lease lives in the room, so making the room is a write no lease can cover.
// Bootstrap, OpenLeaseLock and WriteBuildRecord are the only writes that work
// without one, and build.Acquire is their one caller, which a test over the
// module holds. None of them notes a completed write for the shell's
// observation: making a room or taking its lease is no mail operation, and a
// refused call must not leave evidence that the state was written.

// Bootstrap makes the base, the root, the room and the room's sessions, inbox
// and sock, each when missing, and verifies each that exists without repairing
// its mode: a directory somebody else could write is refused, not chmodded. Each
// level it makes is synced into its parent, and a failed sync fails the
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

// makeLevel makes one directory of the bootstrap, or verifies the one there.
func makeLevel(path string) error {
	if _, err := os.Lstat(path); err == nil {
		return Verify(path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return noteReach(fmt.Errorf("could not read the state directory %s: %w", path, err), false)
	}
	Step("bootstrap " + filepath.Base(path))
	err := os.Mkdir(path, dirMode)
	if errors.Is(err, os.ErrExist) {
		// Made by another process meanwhile: verified like any other.
		return Verify(path)
	}
	if err != nil {
		return noteReach(fmt.Errorf("could not create the state directory %s: %w", path, err), false)
	}
	if err := fault(OpWrite, path); err != nil {
		return err
	}
	if err := syncLevel(filepath.Dir(path)); err != nil {
		return noteReach(fmt.Errorf("could not flush the state directory %s: %w", filepath.Dir(path), err), false)
	}
	return Verify(path)
}
