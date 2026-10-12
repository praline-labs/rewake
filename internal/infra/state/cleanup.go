package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
)

// The cleanup capability (docs/v2/stage3-upgrade.md#the-cleanup-authority):
// clearing the state of rewake 1.x renames trees beside the root and locks
// v2/.clear.lock, none of which lies in a room, so a room's lease authorizes
// none of it — the guard refuses those paths like any outside every room. A
// launch holding its room's lease takes this capability from infra/build for
// exactly these names, and lets it go before its claim. It covers the base it
// was taken for and nothing else in it: the 1.x tree rooms/, the trees a
// clearer set aside beside it, the clear lock and the counts a clearing
// records under the root.

// Names a clearing gives and takes, directly under the base or the root.
const (
	legacyRooms     = "rooms"
	candidatePrefix = ".rooms-1x-candidate-"
	validatedPrefix = ".rooms-1x-validated-"
	clearLockName   = ".clear.lock"
	summaryPrefix   = ".cleared-1x-"
	summarySuffix   = ".json"
	legacyLaunch    = ".launch.lock"
)

// OpSync is the flush of a directory a clearing's rename waits for: a
// durable step a fault plan counts and may fail, since a rename counts only
// once its flush succeeded.
const OpSync = "sync"

// treeID is a clearer's id for the tree it sets aside: its pid, its start
// and a random part, so that it cannot recur across boots.
var treeID = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9a-f]{16}$`)

// ValidTreeID says whether id may name a tree a clearer set aside.
func ValidTreeID(id string) bool { return treeID.MatchString(id) }

// ErrClearBusy means the clear lock was not taken within the caller's bound:
// another launch is checking the state of 1.x.
var ErrClearBusy = errors.New("another launch is checking the state of rewake 1.x")

// ErrLegacyBusy means a lock of 1.x was not taken within its bound: a 1.x
// command holds it.
var ErrLegacyBusy = errors.New("a lock of rewake 1.x is held")

// Cleanup is a launch's capability over the 1.x trees of one base, valid
// while the room lease it was taken under stays authorized and until Close.
type Cleanup struct {
	base string
	lock *LeaseLock

	mu     sync.Mutex
	closed bool
}

// AuthorizeCleanup gives the holder of a room's lease the cleanup of that
// room's base. infra/build alone calls it, as it alone makes a room: a test
// over the module fails on any other reference.
func AuthorizeCleanup(l *LeaseLock) (*Cleanup, error) {
	if !authorized(l) {
		return nil, &os.PathError{Op: OpRename, Path: l.room, Err: ErrNoLease}
	}
	base, err := BaseForRoom(l.room)
	if err != nil {
		return nil, err
	}
	return &Cleanup{base: base, lock: l}, nil
}

// authorized says whether this very lock is the one Authorize recorded.
func authorized(l *LeaseLock) bool {
	leases.mu.Lock()
	defer leases.mu.Unlock()
	return slices.Contains(leases.held[l.room], l)
}

// Base is the base the capability covers.
func (c *Cleanup) Base() string { return c.base }

// Close ends the capability; whatever it took stays the caller's to close.
func (c *Cleanup) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
}

// check refuses an act of the cleanup in view mode, without the lease it was
// taken under or after Close, and on any path outside its names.
func (c *Cleanup) check(op, path string) error {
	if err := viewWrite(op, path); err != nil {
		return err
	}
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed || !authorized(c.lock) {
		return refuseNoLease(op, path)
	}
	if !c.covers(path) {
		return &os.PathError{Op: op, Path: path, Err: fmt.Errorf("outside the trees of rewake 1.x in %s", c.base)}
	}
	return nil
}

// covers says whether path is one of the cleanup's: in rooms/ or a tree set
// aside, spelled clean, or the clear lock or a summary under the root.
func (c *Cleanup) covers(path string) bool {
	if path != filepath.Clean(path) {
		return false
	}
	rel, err := filepath.Rel(c.base, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return false
	}
	first, rest, _ := strings.Cut(rel, string(filepath.Separator))
	switch {
	case first == legacyRooms:
		return true
	case strings.HasPrefix(first, candidatePrefix):
		return ValidTreeID(strings.TrimPrefix(first, candidatePrefix))
	case strings.HasPrefix(first, validatedPrefix):
		return ValidTreeID(strings.TrimPrefix(first, validatedPrefix))
	case first == rootName:
		return rest == clearLockName || summaryName(rest)
	}
	return false
}

func summaryName(name string) bool {
	id, ok := strings.CutPrefix(name, summaryPrefix)
	if !ok {
		return false
	}
	id, ok = strings.CutSuffix(id, summarySuffix)
	return ok && ValidTreeID(id)
}

// TreeLock is a lock the cleanup took: the clear lock, or a 1.x room's
// .launch.lock, held exclusive until Close.
type TreeLock struct {
	file *os.File
	path string
}

// Path is where the lock was taken.
func (l *TreeLock) Path() string { return l.path }

// At says whether the held file is the one at the given path now: after the
// room it locks was renamed, the descriptor taken before is checked against
// the renamed path rather than opened again, which would contend with itself.
func (l *TreeLock) At(path string) (bool, error) {
	var open, named syscall.Stat_t
	if err := syscall.Fstat(int(l.file.Fd()), &open); err != nil {
		return false, err
	}
	if err := syscall.Lstat(path, &named); err != nil {
		if errors.Is(err, syscall.ENOENT) {
			return false, nil
		}
		return false, err
	}
	return open.Dev == named.Dev && open.Ino == named.Ino, nil
}

// Close lets the lock go.
func (l *TreeLock) Close() {
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
}

// LockClear takes the clear lock, <root>/.clear.lock, exclusive, waiting at
// most wait; past it ErrClearBusy.
func (c *Cleanup) LockClear(wait time.Duration) (*TreeLock, error) {
	return c.lockFile(filepath.Join(c.base, rootName, clearLockName), wait, ErrClearBusy)
}

// LockRoom takes the .launch.lock of a 1.x room — in rooms/ or a tree set
// aside — exclusive, waiting at most wait; past it ErrLegacyBusy. The file is
// created when missing, so a 1.x launch that opens it later finds this inode.
func (c *Cleanup) LockRoom(room string, wait time.Duration) (*TreeLock, error) {
	if !c.legacyRoom(room) {
		return nil, &os.PathError{Op: opLock, Path: room, Err: errors.New("not a room of a tree of rewake 1.x")}
	}
	for _, level := range []string{filepath.Dir(room), room} {
		if err := plainDir(level); err != nil {
			return nil, err
		}
	}
	return c.lockFile(filepath.Join(room, legacyLaunch), wait, ErrLegacyBusy)
}

// legacyRoom says whether room is directly under rooms/ or a candidate tree.
func (c *Cleanup) legacyRoom(room string) bool {
	tree := filepath.Base(filepath.Dir(room))
	return filepath.Dir(filepath.Dir(room)) == c.base && c.covers(room) &&
		(tree == legacyRooms || strings.HasPrefix(tree, candidatePrefix))
}

func (c *Cleanup) lockFile(path string, wait time.Duration, busy error) (*TreeLock, error) {
	if err := c.check(opLock, path); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	for attempt := 0; attempt < leaseLockAttempts; attempt++ {
		file, err := openRegular(path, os.O_RDWR|os.O_CREATE)
		if err != nil {
			return nil, fmt.Errorf("could not open %s: %w", path, err)
		}
		lock := &TreeLock{file: file, path: path}
		if err := flockBy(file, deadline); err != nil {
			lock.Close()
			if errors.Is(err, ErrLeaseBusy) {
				return nil, fmt.Errorf("%w: %s", busy, path)
			}
			return nil, fmt.Errorf("could not lock %s: %w", path, err)
		}
		// Replaced while this descriptor waited: its lock locks nothing anyone
		// else opens.
		if same, err := lock.At(path); err != nil || same {
			if err != nil {
				lock.Close()
				return nil, err
			}
			return lock, nil
		}
		lock.Close()
	}
	return nil, fmt.Errorf("%s was replaced %d times while it was locked", path, leaseLockAttempts)
}

// flockBy takes an exclusive flock, trying again until deadline.
func flockBy(file *os.File, deadline time.Time) error {
	for {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EINTR {
			return err
		}
		if !time.Now().Before(deadline) {
			return ErrLeaseBusy
		}
		time.Sleep(leasePoll)
	}
}

// TryLock asks whether a 1.x lock file that exists — a name's or a
// mailbox's — is free: taken without waiting and let go at once, a probe and
// no hold. A missing file holds nothing; a lock not taken is false.
func (c *Cleanup) TryLock(path string) (bool, error) {
	if err := c.check(opLock, path); err != nil {
		return false, err
	}
	file, err := openRegular(path, os.O_RDONLY)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("could not open %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("could not lock %s: %w", path, err)
	}
	return true, nil
}

// openRegular opens a lock file of the cleanup, created when flags say so,
// and refuses anything but a regular file: never following a link, and never
// waiting in open on a FIFO, which would hold the clear lock and every room
// lock taken until some writer came. A special file is refused before it is
// opened, and the descriptor checked again after, against a swap between.
func openRegular(path string, flags int) (*os.File, error) {
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	file, err := os.OpenFile(path, flags|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0o600)
	if err != nil {
		return nil, err
	}
	if info, err := file.Stat(); err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		if err == nil {
			err = fmt.Errorf("%s is not a regular file", path)
		}
		return nil, err
	}
	return file, nil
}

// ReadPlainDir lists a directory of its own in name order. The open takes
// nothing but a directory and follows no link, so a FIFO or a link put where
// a directory was checked is refused by the open itself — never waited on,
// never followed.
func ReadPlainDir(path string) ([]os.DirEntry, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_CLOEXEC|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, syscall.ENOTDIR) || errors.Is(err, syscall.ELOOP) {
		return nil, fmt.Errorf("%s is not a directory", path)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	entries, err := file.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(entries, func(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return entries, nil
}

// plainDir refuses a path that is not a directory of its own: a symlink is
// never followed out of the base.
func plainDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	return nil
}
