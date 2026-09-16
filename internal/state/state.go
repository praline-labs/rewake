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
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"
	"time"
)

// DirEnv names the environment variable that overrides the state directory.
// The wrapper passes it to the harness so a sandbox with a different TMPDIR
// still finds the same directory.
const DirEnv = "REWAKE_DIR"

// SessionEnv names the environment variable carrying the session's own name.
// The name avoids KEY, SECRET and TOKEN: Codex strips such variables from the
// environment of the commands its agent runs.
const SessionEnv = "REWAKE_SESSION"

// EpochEnv names the variable carrying the run of that name, the session
// record's epoch. A name outlives its session; a process left behind by one run
// must not read or answer for the next.
const EpochEnv = "REWAKE_EPOCH"

// dirMode is used for every directory rewake creates.
const dirMode os.FileMode = 0o700

// nameShape is what a session name may look like. It becomes a file name and an
// address, so it stays short, lower case and free of separators.
var nameShape = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,31}$`)

// ValidName reports whether a name may address a session.
func ValidName(name string) bool { return nameShape.MatchString(name) }

// Root returns the shared state root, without reading legacy records there.
func Root() (string, error) {
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

	return path, nil
}

const (
	sessionsDir  = "sessions"
	inboxDir     = "inbox"
	socketsDir   = "sock"
	doneDir      = "done"
	unreadDir    = "unread"
	awaitingDir  = "awaiting"
	answeringDir = "answering"
)

// SessionPath is the record of one session.
func SessionPath(dir, name string) string {
	return filepath.Join(dir, sessionsDir, name+".json")
}

// SessionsPath is the directory holding every session record.
func SessionsPath(dir string) string { return filepath.Join(dir, sessionsDir) }

// InboxPath is the mailbox of one session.
func InboxPath(dir, name string) string { return filepath.Join(dir, inboxDir, name) }

// DonePath holds messages that have been read or refused, for diagnosis.
func DonePath(dir, name string) string { return filepath.Join(dir, inboxDir, name, doneDir) }

// UnreadPath holds messages the session has been told about and has not read
// yet. The harness is handed a notice, not the text: the agent fetches the text
// itself, so it knows the message came through a tool and not from its user.
func UnreadPath(dir, name string) string { return filepath.Join(dir, inboxDir, name, unreadDir) }

// AnsweringPath holds one mark per question a send in this session is blocked
// on. While a mark is fresh, the answer is handed to that send rather than
// announced to the agent.
func AnsweringPath(dir, name string) string {
	return filepath.Join(dir, inboxDir, name, answeringDir)
}

// AwaitingPath lists the sessions whose messages this session has read since its
// last turn ended. Each of them is told when that turn ends.
func AwaitingPath(dir, name string) string {
	return filepath.Join(dir, inboxDir, name, awaitingDir)
}

// SocketPath is where the wrapper asks a harness to put its inbox socket. It is
// kept short: a unix socket path may not exceed 103 bytes.
//
// The path carries the run as well as the name. A name changes hands, and a
// socket shared by every run of it could be removed by a wrapper on its way out
// just after the next run had bound it; a path of its own belongs to one run.
func SocketPath(dir, name, run string) string {
	basename := name
	if run != "" {
		basename += "." + run
	}
	path := filepath.Join(dir, socketsDir, basename+".sock")
	if len(path) > 103 {
		sum := sha256.Sum256([]byte(name + "\x00" + run))
		path = filepath.Join(dir, socketsDir, fmt.Sprintf("%x.sock", sum[:12]))
	}
	return path
}

// ensureDir creates a directory and refuses one that somebody else could write.
// A new directory is flushed to its parent: a mailbox that exists only in the
// page cache takes messages that a crash then takes away.
func ensureDir(path string) error {
	_, err := os.Stat(path)
	fresh := err != nil

	if err := os.MkdirAll(path, dirMode); err != nil {
		return fmt.Errorf("could not create the state directory %s: %w", path, err)
	}
	if fresh {
		_ = syncDir(filepath.Dir(path))
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
// content or the new one, never a half-written file. The content is flushed
// before it is published and the directory after, so a crash cannot leave a
// name pointing at bytes that were never written.
func WriteAtomic(path string, data []byte) error {
	name, err := writeTemp(filepath.Dir(path), data)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(name) }()

	if err := os.Rename(name, path); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}

// writeTemp writes data to a new file in dir and returns its name.
func writeTemp(dir string, data []byte) (string, error) {
	temp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return "", err
	}
	name := temp.Name()

	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	if err := os.Chmod(name, 0o600); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}

// SyncDir flushes a directory entry, so a rename or a link survives a crash.
func SyncDir(path string) error { return syncDir(path) }

// syncDir flushes a directory entry, so a rename or a link survives a crash.
func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	return dir.Sync()
}

// ErrNameTaken is returned when a name is already published.
var ErrNameTaken = errors.New("name taken")

// PublishExclusive writes a file only if the name is free. It writes a temporary
// file and links it into place: link fails when the target exists, so two
// processes claiming one name cannot both believe they won.
func PublishExclusive(path string, data []byte) error {
	dir := filepath.Dir(path)
	name, err := writeTemp(dir, data)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(name) }()

	if err := os.Link(name, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrNameTaken
		}
		return err
	}
	return syncDir(dir)
}

// WithNameLock runs fn while holding an exclusive lock on one session name.
//
// Claiming a name is not a single step when the previous owner is gone: the
// record has to be read, judged dead and removed before a new one is linked in.
// Without a lock two claimants can both pass that sequence, each deleting what
// the other just published, and end up serving one mailbox from two processes.
// The lock is an open file plus flock, so it is released even if the process is
// killed, and a leftover lock file locks nothing.
func WithNameLock(dir, name string, fn func() error) error {
	return withLock(filepath.Join(SessionsPath(dir), "."+name+".lock"), "the name "+name, fn)
}

// ErrMailboxBusy means the mailbox lock was held by somebody else for longer
// than the caller was willing to wait.
var ErrMailboxBusy = errors.New("the mailbox is busy")

// LockUnusableError means the mailbox lock could not be taken at all — its file
// cannot be opened or locked. Nobody can hold such a lock, which is what lets
// the one process that must keep working carry on without it.
type LockUnusableError struct{ Err error }

func (e *LockUnusableError) Error() string {
	return "the mailbox lock cannot be used: " + e.Err.Error()
}
func (e *LockUnusableError) Unwrap() error { return e.Err }

// WithMailboxLock runs fn while holding the lock of one mailbox. Every change to
// a message's state — making it readable, recording what delivery did, reading
// it, reporting a turn — happens under it: those are separate processes, and
// two of them acting on the same message at once is how a read task was handed
// out a second time. The lock is not reentrant: fn must not take it again.
//
// The wait ends with ctx, as ErrMailboxBusy. A holder can be stuck for as long
// as a reader's stdout is — a pager, a pipe nobody drains — and waiting without
// an end let that stop delivery, the end of turns and the wrapper's own exit.
func WithMailboxLock(ctx context.Context, dir, name string, fn func() error) error {
	mailbox := InboxPath(dir, name)
	if err := EnsureSubdir(mailbox); err != nil {
		return &LockUnusableError{Err: err}
	}
	path := filepath.Join(mailbox, ".lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return &LockUnusableError{Err: err}
	}
	defer func() { _ = file.Close() }()

	for {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EINTR {
			return &LockUnusableError{Err: err}
		}
		select {
		case <-ctx.Done():
			return ErrMailboxBusy
		case <-time.After(lockPoll):
		}
	}
	defer func() { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN) }()
	return fn()
}

// lockPoll is how often a busy mailbox lock is tried again. Holders keep it for
// a file operation or two, so the wait is short in the common case.
const lockPoll = 10 * time.Millisecond

func withLock(path, what string, fn func() error) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("could not lock %s: %w", what, err)
	}
	defer func() { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN) }()

	return fn()
}
