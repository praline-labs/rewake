/*
Package state owns the directory rewake keeps its sessions and mailboxes in.

The directory lives in /tmp on purpose. A harness's sandbox may write there by
default, while $XDG_RUNTIME_DIR is out of its reach — and an agent that cannot
write a message file cannot talk to anyone. Everything inside is per-user and
mode 0700, which is the same trust boundary the harnesses use for their own
sockets: anything running as this user can read and write it.
*/
package state

import (
	"context"
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
// The name avoids KEY, SECRET and TOKEN: a harness may strip such variables
// from the environment of the commands its agent runs.
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

// Base is the state directory as the environment names it, checked and made
// nowhere: the build lease is taken from it before anything is created.
func Base() (string, error) {
	path := os.Getenv(DirEnv)
	if path == "" {
		path = filepath.Join(os.TempDir(), "rewake-"+strconv.Itoa(os.Getuid()))
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%s must be an absolute path, got %q", DirEnv, path)
	}
	return path, nil
}

// rootName is the level of the base that holds 2.0's state
// (docs/rules/state.md#the-roots): 1.x keeps its rooms beside it, in the
// base itself, and neither reads the other's.
const rootName = "v2"

// Root is the state root, <base>/v2, made nowhere: only a launch's bootstrap
// creates it. Each level that exists is verified.
func Root() (string, error) {
	base, err := Base()
	if err != nil {
		return "", err
	}
	root := filepath.Join(base, rootName)
	for _, level := range []string{base, root} {
		if err := verifyPresent(level); err != nil {
			return "", err
		}
	}
	return root, nil
}

// verifyPresent verifies a level of the state that exists and passes one that
// does not: a resolver says where the state is, and a missing level is the
// state nobody made yet, not an error.
func verifyPresent(path string) error {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return Verify(path)
}

// ensureDir creates a directory and refuses one that somebody else could write.
// A new directory is flushed to its parent: a mailbox that exists only in the
// page cache takes messages that a crash then takes away.
func ensureDir(path string) error {
	if err := guard(OpWrite, path); err != nil {
		return err
	}
	return makeDir(path)
}

func makeDir(path string) error {
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

// TryWithNameLock runs fn only when the name lock is free right now, and says
// whether it ran. It is for work that can be left to the next caller — a
// change form pruning a dead record it came across — which must not wait on a
// holder it cannot see. A view prunes nothing and takes no lock.
func TryWithNameLock(dir, name string, fn func() error) (bool, error) {
	return tryLock(filepath.Join(SessionsPath(dir), "."+name+".lock"), "the name "+name, fn)
}

// WithSpaceLock runs fn while holding the lock of the room's space records.
// A change of a record reads it, checks the step and writes it again, and the
// open of a name looks at every record first; two writers between those steps
// would each act on what the other was replacing. The directory is made
// when missing, under the same guard as the lock.
func WithSpaceLock(dir string, fn func() error) error {
	if err := guard(opLock, SpacesPath(dir)); err != nil {
		return err
	}
	if err := ensureDir(SpacesPath(dir)); err != nil {
		return err
	}
	return withLock(filepath.Join(SpacesPath(dir), ".lock"), "the room's spaces", fn)
}

// WithBriefLock runs fn while holding the lock of the room's briefs. An id is
// the highest plus one, a log entry's number too, and a tick checks the
// brief's edition and bindings before it adds its entry: two writers between
// those steps would take one number or act on what the other was changing.
// It is taken after the lease with no other lock of the room held, and
// nothing is taken inside it. The directory is made when missing, under the
// same guard as the lock.
func WithBriefLock(dir string, fn func() error) error {
	if err := guard(opLock, BriefsPath(dir)); err != nil {
		return err
	}
	if err := ensureDir(BriefsPath(dir)); err != nil {
		return err
	}
	return withLock(filepath.Join(BriefsPath(dir), ".lock"), "the room's briefs", fn)
}

// ErrMailboxBusy means the mailbox lock was held by somebody else for longer
// than the caller was willing to wait.
var ErrMailboxBusy = errors.New("the mailbox is busy")

// LockUnusableError means the mailbox lock could not be taken at all — its file
// cannot be opened or locked. That says nothing of a holder that opened it
// earlier: a descriptor opened before the file's mode changed keeps its lock.
// So carrying on without it is safe only for writes that move and remove
// nothing.
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
	return WithMailboxLockAt(ctx, InboxPath(dir, name), fn)
}

// WithMailboxLockAt is WithMailboxLock for a mailbox named by its path.
func WithMailboxLockAt(ctx context.Context, mailbox string, fn func() error) error {
	if err := guard(opLock, mailbox); err != nil {
		return err
	}
	if err := EnsureSubdir(mailbox); err != nil {
		return &LockUnusableError{Err: err}
	}
	path := filepath.Join(mailbox, ".lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return &LockUnusableError{Err: err}
	}
	defer func() { _ = file.Close() }()

	for waited := false; ; waited = true {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EINTR {
			return &LockUnusableError{Err: err}
		}
		if !waited && LockWait != nil {
			LockWait(mailbox)
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

// LockWait is told the mailbox whose lock a caller found held, once per
// wait, before it waits: a test orders another step against a section only by
// knowing the section has made someone wait. Nil but in tests.
var LockWait func(mailbox string)

// lockPoll is how often a busy mailbox lock is tried again. Holders keep it for
// a file operation or two, so the wait is short in the common case.
const lockPoll = 10 * time.Millisecond

func tryLock(path, what string, fn func() error) (bool, error) {
	if err := guard(opLock, path); err != nil {
		return false, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return false, err
	}
	defer func() { _ = file.Close() }()

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return false, nil
		}
		return false, fmt.Errorf("could not lock %s: %w", what, err)
	}
	defer func() { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN) }()

	return true, fn()
}

func withLock(path, what string, fn func() error) error {
	if err := guard(opLock, path); err != nil {
		return err
	}
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
