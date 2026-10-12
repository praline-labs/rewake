package state

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// FlushFault is asked before every flush this package makes — a temporary
// file's contents before it is linked or renamed into place, a directory's
// entries after, a record's contents (SyncFile) — with the path flushed. A
// test makes a flush fail with it, and learns what a link that is visible
// and not durable leads to: the publication answers an error and the name
// stays taken. It is apart from Fault, whose steps a fault plan counts: a
// flush is part of the step it ends, not a step of its own. Nil in every
// build but a test's.
var FlushFault func(path string) error

func flushFault(path string) error {
	if FlushFault == nil {
		return nil
	}
	return FlushFault(path)
}

// SyncFile flushes the contents of a regular file of the state directory, so
// a record linked into place is durable as it reads. A link is not followed
// and a FIFO is not waited on: either is ErrNotRegular, as for ReadRegular.
func SyncFile(path string) error {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, syscall.ELOOP) {
			return &os.PathError{Op: "open", Path: path, Err: fmt.Errorf("%w: a link", ErrNotRegular)}
		}
		return err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return &os.PathError{Op: "sync", Path: path, Err: fmt.Errorf("%w: %s", ErrNotRegular, info.Mode().Type())}
	}
	if err := flushFault(path); err != nil {
		return err
	}
	return file.Sync()
}
