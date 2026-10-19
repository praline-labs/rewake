package state

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RoomEnv carries the command's inherited conversation namespace.
const RoomEnv = "REWAKE_ROOM"

// DefaultRoom is the room every launch took before rooms were named by their
// directory; a state directory may still hold it, and tests use the name.
const DefaultRoom = "default"

// roomDirectoryFile keeps, in a room's directory, the launch directory the
// room is named for, so a listing can show it.
const roomDirectoryFile = "directory"

// The parts of a room's name: the directory's last element cut to slugLength,
// a dash, and hashLength hex digits of the resolved path, 32 in all.
const (
	slugLength = 19
	hashLength = 12
)

// RoomOf is the room of a launch directory (the owner's decision, October
// 19, 2026): the directory resolved with symlinks followed names it, so two
// launches in one directory share a room and two directories never do. The
// name keeps the directory's last element readable and a hash of the whole
// path, so checkouts named alike stay apart. It returns the resolved path too.
func RoomOf(directory string) (string, string, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return "", "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", "", fmt.Errorf("the room is named by the launch directory, and %s does not resolve: %w", absolute, err)
	}
	sum := sha256.Sum256([]byte(resolved))
	return slug(filepath.Base(resolved)) + "-" + hex.EncodeToString(sum[:])[:hashLength], resolved, nil
}

// slug is a name's letters and digits in lower case, every other run of
// characters one dash, cut to slugLength; "dir" when nothing is left.
func slug(name string) string {
	var out []byte
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out = append(out, byte(r))
		case len(out) > 0 && out[len(out)-1] != '-':
			out = append(out, '-')
		}
	}
	if len(out) > slugLength {
		out = out[:slugLength]
	}
	if trimmed := strings.Trim(string(out), "-"); trimmed != "" {
		return trimmed
	}
	return "dir"
}

// CurrentRoom is the room a command acts in: the one its session inherited,
// or outside a session the room of its working directory.
func CurrentRoom() (string, error) {
	if room := os.Getenv(RoomEnv); room != "" {
		return room, nil
	}
	working, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("the room is named by the working directory, which cannot be read: %w", err)
	}
	room, _, err := RoomOf(working)
	return room, err
}

// RecordRoomDirectory writes the directory a room is named for into it, once:
// the name is a hash of that path, so the content never changes.
func RecordRoomDirectory(roomDir, resolved string) error {
	path := filepath.Join(roomDir, roomDirectoryFile)
	if _, err := os.Lstat(path); err == nil {
		return nil
	}
	return WriteAtomic(path, []byte(resolved+"\n"))
}

// RoomDirectory is the directory a room is named for, or empty for a room
// without the record: one of an earlier build, or a test's.
func RoomDirectory(roomDir string) string {
	raw, err := ReadRegular(filepath.Join(roomDir, roomDirectoryFile), 4096)
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(string(raw), "\n")
}

// Dir returns only this command's room, made nowhere. Legacy records are
// ignored; no compatibility fallback can accidentally join isolated
// conversations.
func Dir() (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}
	room, err := CurrentRoom()
	if err != nil {
		return "", err
	}
	return RoomDir(root, room)
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
