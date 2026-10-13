package state

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// scene is what writePrimitives set up for its writes: a file, the same
// file opened for a lock, a counter holding 3 and a bound socket.
type scene struct {
	file      string
	locked    *os.File
	counter   *Counter
	listening *Listener
}

// leaseFree are the writes that need no lease: the bootstrap's, which only
// view mode refuses (docs/v2/stage3-state.md#the-bootstrap).
var leaseFree = map[string]bool{"Bootstrap": true, "OpenLeaseLock": true, "CreateLeaseLock": true}

// writePrimitives sets up a scene in the room at dir, which the caller holds
// the lease of, and returns every write primitive of the package by name, each
// as one call that writes there. The tests of view mode and of the lease run
// the same list, so a primitive one of them holds the other holds too.
func writePrimitives(t *testing.T, base, dir string) (map[string]func() error, scene) {
	t.Helper()
	file := filepath.Join(dir, "file")
	if err := WriteAtomic(file, []byte("before")); err != nil {
		t.Fatal(err)
	}
	nothing := func() error { return nil }
	locked, err := OpenFile(file, os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = locked.Close() })
	counter, err := OpenCounter(filepath.Join(dir, "counter"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = counter.Close() })
	if err := counter.Store(3); err != nil {
		t.Fatal(err)
	}
	listening, err := Listen(filepath.Join(dir, "listening.sock"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listening.Close() })
	closed := func(f *os.File, err error) error {
		if f != nil {
			_ = f.Close()
		}
		return err
	}
	writes := map[string]func() error{
		"WriteAtomic":      func() error { return WriteAtomic(file, []byte("view")) },
		"PublishExclusive": func() error { return PublishExclusive(filepath.Join(dir, "new"), nil) },
		"WriteAtomicHeld":  func() error { return closed(WriteAtomicHeld(filepath.Join(dir, "held"), nil)) },
		"Remove":           func() error { return Remove(file) },
		"Rename":           func() error { return Rename(file, filepath.Join(dir, "moved")) },
		"EnsureSubdir":     func() error { return EnsureSubdir(filepath.Join(dir, "sub")) },
		"WithNameLock":     func() error { return WithNameLock(dir, "api", nothing) },
		"WithRoomLock":     func() error { return WithRoomLock(dir, nothing) },
		"WithSpaceLock":    func() error { return WithSpaceLock(dir, nothing) },
		"TryWithNameLock":  func() error { _, err := TryWithNameLock(dir, "api", nothing); return err },
		"WithMailboxLock": func() error {
			return WithMailboxLock(context.Background(), dir, "api", nothing)
		},
		"WithMailboxLockAt": func() error {
			return WithMailboxLockAt(context.Background(), filepath.Join(dir, "sessions", "api"), nothing)
		},
		"OpenFile": func() error {
			return closed(OpenFile(filepath.Join(dir, "opened"), os.O_CREATE|os.O_WRONLY, 0o600))
		},
		"OpenFile create": func() error {
			return closed(OpenFile(filepath.Join(dir, "created"), os.O_CREATE|os.O_RDONLY, 0o600))
		},
		"WriteFile": func() error { return WriteFile(filepath.Join(dir, "plain"), nil, 0o600) },
		"Link":      func() error { return Link(file, filepath.Join(dir, "linked")) },
		"Move":      func() error { return Move(file, filepath.Join(dir, "moved")) },
		"Chmod":     func() error { return Chmod(file, 0o644) },
		"Touch":     func() error { return Touch(file, time.Unix(0, 0)) },
		"Discard":   func() error { return Discard(file) },
		"RemoveAll": func() error { return RemoveAll(filepath.Join(dir, "sessions")) },
		"Flock":     func() error { return Flock(locked, syscall.LOCK_EX|syscall.LOCK_NB) },
		"OpenCounter": func() error {
			opened, err := OpenCounter(filepath.Join(dir, "another counter"))
			if opened != nil {
				_ = opened.Close()
			}
			return err
		},
		"Counter.Store": func() error { return counter.Store(7) },
		"Counter.Sync":  func() error { return counter.Sync() },
		"Listen": func() error {
			listener, err := Listen(filepath.Join(dir, "another.sock"))
			if listener != nil {
				_ = listener.Close()
			}
			return err
		},
		"Listener.Close":  func() error { return listening.Close() },
		"Bootstrap":       func() error { _, err := Bootstrap(base, "blue"); return err },
		"OpenLeaseLock":   func() error { return lockClosed(OpenLeaseLock(dir)) },
		"CreateLeaseLock": func() error { return lockClosed(CreateLeaseLock(dir)) },
	}
	return writes, scene{file: file, locked: locked, counter: counter, listening: listening}
}

// lockClosed closes a lease lock a primitive opened and answers its error.
func lockClosed(lock *LeaseLock, err error) error {
	if lock != nil {
		lock.Close()
	}
	return err
}
