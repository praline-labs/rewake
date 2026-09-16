package state

import (
	"fmt"
	"os"
	"path/filepath"
)

// RoomEnv carries the command's inherited conversation namespace.
const RoomEnv = "REWAKE_ROOM"

// DefaultRoom is used by shells and launches without an explicit room.
const DefaultRoom = "default"

// CurrentRoom is inherited by commands. Launches deliberately default to the
// default room instead; only their --room flag selects another one.
func CurrentRoom() string {
	if room := os.Getenv(RoomEnv); room != "" {
		return room
	}
	return DefaultRoom
}

// Dir returns only this command's room. Legacy records in the root are ignored;
// no compatibility fallback can accidentally join isolated conversations.
func Dir() (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}
	return RoomDir(root, CurrentRoom())
}

// RoomDir validates a room and opens its independent sessions and mailboxes.
func RoomDir(root, room string) (string, error) {
	if !ValidName(room) {
		return "", fmt.Errorf("invalid room name %q; use lower-case letters, digits, dot, dash or underscore, up to 32 characters", room)
	}
	if err := Verify(root); err != nil {
		return "", err
	}
	rooms := filepath.Join(root, "rooms")
	if err := EnsureSubdir(rooms); err != nil {
		return "", err
	}
	path := filepath.Join(rooms, room)
	if err := EnsureSubdir(path); err != nil {
		return "", err
	}
	for _, sub := range []string{sessionsDir, inboxDir, socketsDir} {
		if err := EnsureSubdir(filepath.Join(path, sub)); err != nil {
			return "", err
		}
	}
	return path, nil
}

// WithRoomLock serializes role selection and name publication across wrappers.
// The lock is released before harness launch and does not hold for a session's life.
func WithRoomLock(dir string, fn func() error) error {
	return withLock(filepath.Join(dir, ".launch.lock"), "the room launch", fn)
}

// RootForRoom reverses the rooms/<name> namespace without consulting ambient
// environment, so a wrapper always passes its original root to children.
func RootForRoom(directory string) string { return filepath.Dir(filepath.Dir(directory)) }
