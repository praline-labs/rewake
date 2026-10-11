package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTryHoldWaitsForTheWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "record.json")
	writer, err := WriteAtomicHeld(path, []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if held, ok := TryHold(path); ok {
		_ = held.Close()
		t.Fatal("held while its writer holds it")
	}
	_ = writer.Close()
	held, ok := TryHold(path)
	if !ok {
		t.Fatal("not held once its writer let go")
	}
	_ = held.Close()
}

// The writer removes its file and lets go between TryHold's open and its
// lock: the lock is free, on a file no name leads to, and holding it would
// pass a closed record for an open one.
func TestTryHoldRefusesAFileRemovedBeforeItsLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "record.json")
	writer, err := WriteAtomicHeld(path, []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	saved := beforeHold
	beforeHold = func(string) {
		if err := os.Remove(path); err != nil {
			t.Error(err)
		}
		_ = writer.Close()
	}
	t.Cleanup(func() { beforeHold = saved })
	if held, ok := TryHold(path); ok {
		_ = held.Close()
		t.Fatal("held a file removed before its lock")
	}
}

// The same, with another file under the name by the time of the lock: the
// lock taken is not that file's.
func TestTryHoldRefusesAFileReplacedBeforeItsLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "record.json")
	writer, err := WriteAtomicHeld(path, []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	saved := beforeHold
	beforeHold = func(string) {
		_ = writer.Close()
		if err := WriteAtomic(path, []byte("{}")); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(func() { beforeHold = saved })
	if held, ok := TryHold(path); ok {
		_ = held.Close()
		t.Fatal("held a file replaced before its lock")
	}
}
