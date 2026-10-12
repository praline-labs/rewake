package state

import (
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
)

// Fault is the one test hook of the state directory's file operations
// (docs/mail-bridge-checks.md#testing-without-a-live-harness). Every write,
// removal and read that goes through this package asks it first, with the
// operation and the path: a test learns the order of a call's durable steps
// from it, and makes any one of them fail or end the process. Nil in every
// build but a test's.
var Fault func(op, path string) error

// The operations Fault is asked about.
const (
	OpRead    = "read"
	OpWrite   = "write"
	OpPublish = "publish"
	OpRemove  = "remove"
	OpRename  = "rename"
	// OpStep is a step of a call's path that is no file operation: the
	// server asking for a ticket, starting a child, writing an answer.
	OpStep = "step"
	// OpNote marks a point a test observes in its own process, which a
	// fault plan logs and never counts: the lease's release, after the last
	// step of every invocation, where ending the process is no new case.
	OpNote = "note"
)

func fault(op, path string) error {
	if Fault == nil {
		return nil
	}
	return Fault(op, path)
}

// ReadFile reads a file of the state directory.
func ReadFile(path string) ([]byte, error) {
	if err := fault(OpRead, path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	return data, noteReach(err, false)
}

// ErrNotRegular is a read of a path that is no regular file: a link, a
// directory, a FIFO, a device or a socket.
var ErrNotRegular = errors.New("not a regular file")

// ReadRegular reads a regular file of the state directory, of at most limit
// bytes, and nothing else. A link is not followed, a FIFO is not waited on
// for a writer and a file past limit is not read whole: each is an error,
// ErrNotRegular or the bound, apart from a missing file, which is
// os.ErrNotExist as for ReadFile. The type is checked on the file opened, so
// a name swapped between a listing and the read is refused as it is now.
func ReadRegular(path string, limit int64) ([]byte, error) {
	if err := fault(OpRead, path); err != nil {
		return nil, err
	}
	data, err := readRegular(path, limit)
	return data, noteReach(err, false)
}

func readRegular(path string, limit int64) ([]byte, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if errors.Is(err, syscall.ELOOP) {
		return nil, &os.PathError{Op: "open", Path: path, Err: fmt.Errorf("%w: a link", ErrNotRegular)}
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, &os.PathError{Op: "read", Path: path, Err: fmt.Errorf("%w: %s", ErrNotRegular, info.Mode().Type())}
	}
	if info.Size() > limit {
		return nil, &os.PathError{Op: "read", Path: path, Err: fmt.Errorf("%d bytes, past the bound of %d", info.Size(), limit)}
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err == nil && int64(len(data)) > limit {
		err = &os.PathError{Op: "read", Path: path, Err: fmt.Errorf("more than the bound of %d bytes", limit)}
	}
	return data, err
}

// Remove removes a file of the state directory.
func Remove(path string) error {
	if err := guard(OpRemove, path); err != nil {
		return err
	}
	if err := fault(OpRemove, path); err != nil {
		return err
	}
	return noteReach(os.Remove(path), true)
}

// Rename moves a file of the state directory to another name in it.
func Rename(from, to string) error {
	if err := guard(OpRename, from, to); err != nil {
		return err
	}
	if err := fault(OpRename, to); err != nil {
		return err
	}
	return noteReach(os.Rename(from, to), true)
}

// AskRead asks Fault about a read of the state directory that another package
// makes with its own reader, so a test can fail it like any read here. It
// reads nothing.
func AskRead(path string) error { return fault(OpRead, path) }

// Step names a step of a call's path that touches no file, so a fault plan
// can end the process there as it ends it before a write. It cannot fail.
func Step(name string) { _ = fault(OpStep, name) }

// Note marks a point of a call's path for a test to observe (OpNote). It
// cannot fail and is no durable step.
func Note(name string) { _ = fault(OpNote, name) }
