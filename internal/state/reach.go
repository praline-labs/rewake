package state

import (
	"errors"
	"sync"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
)

// What this process met reaching the state directory, for the shell
// observation of the mail channel (docs/mail-bridge-channel.md#the-shell-observation):
// the last write that completed and the first error that says the state could
// not be reached — read-only, not permitted, or an I/O error — each with the
// time it happened. A command that refused, timed out on a lock or wrote
// nothing met neither. The times are taken here, as the write completes under
// its lock, never when the command returns: a command may wait long after it
// wrote, and its write is no evidence of anything that began meanwhile.
var reach struct {
	mu   sync.Mutex
	last Reach
}

// Reach is what a process met: a completed write and when the last one
// completed, and the first error reaching the state and when it came.
type Reach struct {
	Wrote    bool
	WroteAt  Moment
	Failed   error
	FailedAt Moment
}

// Moment is a time on both clocks: the boot clock orders, the wall clock is
// shown.
type Moment struct {
	Boot int64
	Wall time.Time
}

func now() Moment { return Moment{Boot: boottime.Now(), Wall: time.Now()} }

// Unreachable says whether an error is one of reaching the state rather
// than a refusal or a wait.
func Unreachable(err error) bool {
	for _, errno := range []syscall.Errno{syscall.EROFS, syscall.EACCES, syscall.EPERM, syscall.EIO} {
		if errors.Is(err, errno) {
			return true
		}
	}
	return false
}

// noteReach records the outcome of one write, or of reaching the state for
// one, and passes the error on unchanged.
func noteReach(err error, write bool) error {
	reach.mu.Lock()
	defer reach.mu.Unlock()
	switch {
	case err == nil && write:
		reach.last.Wrote, reach.last.WroteAt = true, now()
	case err != nil && reach.last.Failed == nil && Unreachable(err):
		reach.last.Failed, reach.last.FailedAt = err, now()
	}
	return err
}

// Reached answers what this process met.
func Reached() Reach {
	reach.mu.Lock()
	defer reach.mu.Unlock()
	return reach.last
}

// ResetReach forgets what was met. Each command starts with it, so what
// one command met is never another's evidence where a process runs many.
func ResetReach() {
	reach.mu.Lock()
	defer reach.mu.Unlock()
	reach.last = Reach{}
}

// NoteReach lets a package beside this one record a lock it could not take
// on a file of the state directory.
func NoteReach(err error) error { return noteReach(err, false) }
