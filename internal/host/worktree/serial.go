package worktree

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Checkouts of one repository are made and removed one at a time
// (docs/worktree.md#launches-at-once). With git 2.43 a git worktree add
// running beside another can read the other's new entry in <common
// dir>/worktrees with its commondir still empty, and dies
// (docs/research-worktree.md). The lock is a
// file beside the repository's directory under the root, not in the
// repository's Git directory: rewake writes nothing of its own into another
// program's metadata. It is not inside the repository's directory either,
// which goes with its last checkout.

// lockWait bounds the wait for another rewake command's checkout of the same
// repository: a git worktree add, the copies .worktreeinclude asks for, a git
// worktree remove. A holder past it is stuck, or copying large ignored
// directories .worktreeinclude names, which stay under the lock so a parallel
// rm cannot take a checkout still being filled; either way a launch waiting
// without end would hang the caller.
var lockWait = time.Minute

// lockPoll is how often a held lock is tried again.
const lockPoll = 20 * time.Millisecond

// BusyError is a repository whose checkouts another rewake command kept locked
// past lockWait.
type BusyError struct {
	Lock string
	// Holder is the process that took the lock last, 0 when unknown.
	Holder int
	Waited time.Duration
}

func (e *BusyError) Error() string {
	holder := "another rewake command"
	if e.Holder > 0 {
		holder = fmt.Sprintf("rewake process %d", e.Holder)
	}
	return fmt.Sprintf("%s has held the worktree lock %s for over %s, and checkouts of one repository are made and removed one at a time; it may be copying large directories .worktreeinclude names, or it may hang: wait and run this again, and end that process only if it does not finish", holder, e.Lock, e.Waited)
}

// lockPath is the lock of the repository whose checkouts are in directory.
func lockPath(directory string) string { return directory + ".lock" }

// withRepositoryLock runs fn holding the lock of the repository whose
// checkouts are in directory, waiting for it at most lockWait. The root the
// lock lies in must exist: Create makes it, Remove goes without a lock when it
// is gone. The lock is an flock, so a holder that dies lets go; its file
// stays, and a leftover file locks nothing.
func withRepositoryLock(directory string, fn func() error) error {
	path := lockPath(directory)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("cannot open the worktree lock: %w", err)
	}
	defer func() { _ = file.Close() }()
	deadline := time.Now().Add(lockWait)
	for {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			return fmt.Errorf("cannot lock %s: %w", path, err)
		}
		if time.Now().After(deadline) {
			return &BusyError{Lock: path, Holder: lockHolder(path), Waited: lockWait}
		}
		time.Sleep(lockPoll)
	}
	defer func() { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN) }()
	// Who holds it, for the refusal of whoever waits too long; the lock does
	// not depend on it.
	if file.Truncate(0) == nil {
		_, _ = file.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	}
	return fn()
}

func lockHolder(path string) int {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		return 0
	}
	return pid
}

// halfWritten matches git's refusal of another worktree's entry it read while
// that entry was being written: fatal: failed to read <common
// dir>/worktrees/<entry>/commondir. The path is matched, not the words, which
// git translates.
var halfWritten = regexp.MustCompile(`worktrees/[^/\s]+/commondir`)

// retryPause is how long an add that met a half-written entry waits before its
// one retry. git writes the entry's files right after one another, so any
// pause ends that window many times over.
const retryPause = 200 * time.Millisecond

// Test hooks: beforeAdd runs right before each git worktree add, under the
// lock; beforeRetry is the pause before the retry.
var (
	beforeAdd   = func(Record) {}
	beforeRetry = func() { time.Sleep(retryPause) }
)

// checkout checks a claimed name's branch out at its path, and takes the claim
// back when git cannot. The lock keeps rewake's own adds apart, but a git
// worktree add rewake does not run — the person's, or a harness's own worktree
// flag — can still be writing its entry, and git then fails with nothing of
// this checkout's left behind. That one failure is tried again once: what it
// met passes in a moment, and a second failure is something else.
func checkout(source, record Record) error {
	add := func() error {
		beforeAdd(record)
		return git(source.Source, "worktree", "add", record.Path, record.Branch)
	}
	err := add()
	if err != nil && halfWritten.MatchString(err.Error()) && !exists(record.Path) {
		beforeRetry()
		err = add()
	}
	if err != nil {
		unclaim(record)
		return fmt.Errorf("git worktree add failed: %w", err)
	}
	return nil
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
