package state

import (
	"os"
	"path/filepath"
	"syscall"
)

// WriteAtomicHeld is WriteAtomic under an exclusive flock taken before the
// file gets its name, so no reader ever finds it unheld while its writer
// lives. The lock stays until the returned file is closed, and the kernel
// drops it when the writer dies, however it dies.
func WriteAtomicHeld(path string, data []byte) (*os.File, error) {
	name, err := writeTemp(filepath.Dir(path), data)
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(name) }()
	held, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(held.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = held.Close()
		return nil, err
	}
	if err := os.Rename(name, path); err != nil {
		_ = held.Close()
		return nil, err
	}
	if err := syncDir(filepath.Dir(path)); err != nil {
		_ = held.Close()
		return nil, err
	}
	return held, nil
}

// TryHold takes the flock of a file WriteAtomicHeld wrote, without waiting:
// false while its writer still holds it, or when the file is gone. Gone
// includes gone between the open and the lock: a writer that removes its file
// and lets go in that instant leaves the lock free on a file no name leads to
// any more, and holding that would pass a closed record for an open one.
func TryHold(path string) (*os.File, bool) {
	held, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	beforeHold(path)
	if syscall.Flock(int(held.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		_ = held.Close()
		return nil, false
	}
	named, err := os.Stat(path)
	if err != nil {
		_ = held.Close()
		return nil, false
	}
	opened, err := held.Stat()
	if err != nil || !os.SameFile(named, opened) {
		_ = held.Close()
		return nil, false
	}
	return held, true
}

// beforeHold runs between TryHold's open and its lock; a test puts the
// writer's removal and release of that instant there.
var beforeHold = func(string) {}
