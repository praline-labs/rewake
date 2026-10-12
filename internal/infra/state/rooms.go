package state

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

// Dir returns only this command's room, made nowhere. Legacy records are
// ignored; no compatibility fallback can accidentally join isolated
// conversations.
func Dir() (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}
	return RoomDir(root, CurrentRoom())
}

// RoomDir is where a room of the root lives and its sessions and mailboxes
// with it. It creates nothing — a launch's bootstrap does — and verifies
// each level that exists.
func RoomDir(root, room string) (string, error) {
	if !ValidName(room) {
		return "", fmt.Errorf("invalid room name %q; use lower-case letters, digits, dot, dash or underscore, up to 32 characters", room)
	}
	path := filepath.Join(root, "rooms", room)
	levels := []string{root, filepath.Dir(path), path}
	for _, sub := range []string{sessionsDir, inboxDir, socketsDir} {
		levels = append(levels, filepath.Join(path, sub))
	}
	for _, level := range levels {
		if err := verifyPresent(level); err != nil {
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

// RootForRoom is the root a room belongs to, without consulting the ambient
// environment, so a wrapper always passes its own root to its children. The
// inverse is checked, never counted: a directory that is not
// <root>/rooms/<room> under a root named v2 — a 1.x room, a path from another
// layout — is an error, never another directory two levels up.
func RootForRoom(directory string) (string, error) {
	rooms := filepath.Dir(directory)
	root := filepath.Dir(rooms)
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory ||
		!ValidName(filepath.Base(directory)) || filepath.Base(rooms) != "rooms" || filepath.Base(root) != rootName {
		return "", fmt.Errorf("%s is not a room of a rewake state directory (<base>/%s/rooms/<room>)", directory, rootName)
	}
	return root, nil
}

// BaseForRoom is the base a room belongs to, what REWAKE_DIR names for it:
// the parent of its checked root.
func BaseForRoom(directory string) (string, error) {
	root, err := RootForRoom(directory)
	if err != nil {
		return "", err
	}
	return filepath.Dir(root), nil
}

// RoomDirs lists the rooms a state directory already has, making none: a
// command looking for sessions everywhere must not leave rooms behind.
func RoomDirs(root string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, "rooms"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() && ValidName(entry.Name()) {
			dirs = append(dirs, filepath.Join(root, "rooms", entry.Name()))
		}
	}
	sort.Strings(dirs)
	return dirs, nil
}

// AllRoomDirs lists every room of a root and refuses what it cannot take
// for one: an entry that is not a directory of a valid name — a link, a file,
// a name rewake never makes — is an error, as is a listing that fails. A
// check that must find a process outside every session cannot pass over an
// entry it did not understand; RoomDirs, for views, does.
func AllRoomDirs(root string) ([]string, error) {
	rooms := filepath.Join(root, "rooms")
	entries, err := os.ReadDir(rooms)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	dirs := make([]string, 0, len(entries))
	for _, entry := range entries {
		path := filepath.Join(rooms, entry.Name())
		if !entry.IsDir() || !ValidName(entry.Name()) {
			return nil, fmt.Errorf("%s is no room", path)
		}
		dirs = append(dirs, path)
	}
	return dirs, nil
}
