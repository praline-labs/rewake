/*
Package state owns the directory rewake keeps its sessions and mailboxes in.

The directory lives in /tmp on purpose. The Codex sandbox may write there by
default, while $XDG_RUNTIME_DIR is out of its reach — and an agent that cannot
write a message file cannot talk to anyone. Everything inside is per-user and
mode 0700, which is the same trust boundary the harnesses use for their own
sockets: anything running as this user can read and write it.
*/
package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

// DirEnv names the environment variable that overrides the state directory.
// The wrapper passes it to the harness so a sandbox with a different TMPDIR
// still finds the same directory.
const DirEnv = "REWAKE_DIR"

// SessionEnv names the environment variable carrying the session's own name.
// The name avoids KEY, SECRET and TOKEN: Codex strips such variables from the
// environment of the commands its agent runs.
const SessionEnv = "REWAKE_SESSION"

// dirMode is used for every directory rewake creates.
const dirMode os.FileMode = 0o700

// nameShape is what a session name may look like. It becomes a file name and an
// address, so it stays short, lower case and free of separators.
var nameShape = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,31}$`)

// ValidName reports whether a name may address a session.
func ValidName(name string) bool { return nameShape.MatchString(name) }

// Dir returns the state directory, creating it when needed.
func Dir() (string, error) {
	path := os.Getenv(DirEnv)
	if path == "" {
		path = filepath.Join(os.TempDir(), "rewake-"+strconv.Itoa(os.Getuid()))
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%s must be an absolute path, got %q", DirEnv, path)
	}
	if err := ensureDir(path); err != nil {
		return "", err
	}
	for _, sub := range []string{sessionsDir, inboxDir, socketsDir} {
		if err := ensureDir(filepath.Join(path, sub)); err != nil {
			return "", err
		}
	}
	return path, nil
}

const (
	sessionsDir = "sessions"
	inboxDir    = "inbox"
	socketsDir  = "sock"
	doneDir     = "done"
)

// SessionPath is the record of one session.
func SessionPath(dir, name string) string {
	return filepath.Join(dir, sessionsDir, name+".json")
}

// SessionsPath is the directory holding every session record.
func SessionsPath(dir string) string { return filepath.Join(dir, sessionsDir) }

// InboxPath is the mailbox of one session.
func InboxPath(dir, name string) string { return filepath.Join(dir, inboxDir, name) }

// DonePath holds messages that have been delivered or refused, for diagnosis.
func DonePath(dir, name string) string { return filepath.Join(dir, inboxDir, name, doneDir) }

// SocketPath is where the wrapper asks a harness to put its inbox socket. It is
// kept short: a unix socket path may not exceed 103 bytes.
func SocketPath(dir, name string) string {
	return filepath.Join(dir, socketsDir, name+".sock")
}

// ensureDir creates a directory and refuses one that somebody else could write.
func ensureDir(path string) error {
	if err := os.MkdirAll(path, dirMode); err != nil {
		return fmt.Errorf("could not create the state directory %s: %w", path, err)
	}
	return Verify(path)
}

// Verify refuses a state directory that is not ours: a symlink, another user's,
// or one open to the group or the world. Messages and socket paths live here,
// so a directory somebody else can write is a way into every session.
func Verify(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("could not read the state directory %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("the state directory %s is a symlink; point %s somewhere else", path, DirEnv)
	}
	if !info.IsDir() {
		return fmt.Errorf("the state directory %s is not a directory", path)
	}
	if owner, ok := ownerUID(info); ok && owner != os.Getuid() {
		return fmt.Errorf("the state directory %s belongs to uid %d, not to you", path, owner)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return fmt.Errorf("the state directory %s is readable or writable by others (mode %04o); fix it with chmod 700 %s", path, perm, path)
	}
	return nil
}

// EnsureSubdir creates a directory inside the state directory.
func EnsureSubdir(path string) error { return ensureDir(path) }

// WriteAtomic replaces a file in one step: a reader sees either the previous
// content or the new one, never a half-written file.
func WriteAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)

	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, 0o600); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// ErrNameTaken is returned when a name is already published.
var ErrNameTaken = errors.New("name taken")

// PublishExclusive writes a file only if the name is free. It writes a temporary
// file and links it into place: link fails when the target exists, so two
// processes claiming one name cannot both believe they won.
func PublishExclusive(path string, data []byte) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)

	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, 0o600); err != nil {
		return err
	}
	if err := os.Link(name, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrNameTaken
		}
		return err
	}
	return nil
}
