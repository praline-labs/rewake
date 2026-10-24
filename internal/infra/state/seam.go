package state

import (
	"os"
	"syscall"
	"time"
)

// The write seam's plain primitives (docs/rules/state.md#what-is-a-writer):
// writes of the room that are neither a replacement by rename nor an exclusive
// publication — a direct write, an open that may create or truncate, a hard
// link, a mode, a touch, a removal that is cleanup — each the os call it
// names behind the same guard as every other write. None asks Fault: where the
// code they replace asked nothing, a call's durable steps stay the ones its
// fault schedules count.

// writeFlags are the open flags that make an open a write.
const writeFlags = os.O_WRONLY | os.O_RDWR | os.O_CREATE | os.O_TRUNC | os.O_APPEND

// OpenFile opens a file of the state directory as os.OpenFile does. An open
// that can write — for writing, creating or truncating — is a write, and the
// file it gives, with whatever lock is taken on it, stays the caller's to
// close inside its scope. A read-only open passes.
func OpenFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	if flag&writeFlags != 0 {
		if err := guard(OpWrite, path); err != nil {
			return nil, err
		}
	}
	return os.OpenFile(path, flag, perm)
}

// WriteFile writes a file of the state directory in place, as os.WriteFile
// does: for content whose reader tolerates a partial write, a mark or a
// temporary file renamed afterwards.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	if err := guard(OpWrite, path); err != nil {
		return err
	}
	return os.WriteFile(path, data, perm)
}

// Link gives a file of the state directory a second name. Both ends are the
// state's: the name it gets and the file it names.
func Link(from, to string) error {
	if err := guard(OpPublish, from, to); err != nil {
		return err
	}
	return os.Link(from, to)
}

// Move renames a file of the state directory as Rename does, but is no step
// a fault plan counts: the rename of a temporary file its caller wrote in
// place, where a crash leaves only that temporary file.
func Move(from, to string) error {
	if err := guard(OpRename, from, to); err != nil {
		return err
	}
	return os.Rename(from, to)
}

// Chmod sets the mode of a file or directory of the state directory.
func Chmod(path string, mode os.FileMode) error {
	if err := guard(OpWrite, path); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

// Touch sets a file's access and modification times, which some of the state
// reads as a heartbeat or a release time.
func Touch(path string, at time.Time) error {
	if err := guard(OpWrite, path); err != nil {
		return err
	}
	return os.Chtimes(path, at, at)
}

// Discard removes a file of the state directory as cleanup: a temporary
// file, a dead record, a stale mark — no step of a call's durable path, so a
// crash before it leaves what the next caller discards again. Remove is the
// durable removal a fault plan counts.
func Discard(path string) error {
	if err := guard(OpRemove, path); err != nil {
		return err
	}
	return os.Remove(path)
}

// RemoveAll removes a directory of the state directory and everything in it,
// as cleanup: a run's own as its run ends or once a later launch has proven
// it dead, the records of runs that ended.
func RemoveAll(path string) error {
	if err := guard(OpRemove, path); err != nil {
		return err
	}
	return os.RemoveAll(path)
}

// Flock takes or releases a flock on a file of the state directory opened
// with OpenFile. Taking one orders writers, so it is a write; letting one go
// always passes.
func Flock(file *os.File, how int) error {
	if how&syscall.LOCK_UN == 0 {
		if err := guard(opLock, file.Name()); err != nil {
			return err
		}
	}
	return syscall.Flock(int(file.Fd()), how)
}
