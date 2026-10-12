package state

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

// In view mode every write primitive refuses and changes nothing, and
// leaving the mode reports each attempt, so a caller that swallowed the
// refusal is still caught. Outside it, they write as before.
func TestEveryWritePrimitiveRefusesInViewMode(t *testing.T) {
	base := filepath.Join(t.TempDir(), "state")
	dir, err := Bootstrap(base, DefaultRoom)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "file")
	if err := WriteAtomic(file, []byte("before")); err != nil {
		t.Fatal(err)
	}
	nothing := func() error { return nil }
	writes := map[string]func() error{
		"WriteAtomic":      func() error { return WriteAtomic(file, []byte("view")) },
		"PublishExclusive": func() error { return PublishExclusive(filepath.Join(dir, "new"), nil) },
		"WriteAtomicHeld": func() error {
			held, err := WriteAtomicHeld(filepath.Join(dir, "held"), nil)
			if held != nil {
				_ = held.Close()
			}
			return err
		},
		"Remove":          func() error { return Remove(file) },
		"Rename":          func() error { return Rename(file, filepath.Join(dir, "moved")) },
		"EnsureSubdir":    func() error { return EnsureSubdir(filepath.Join(dir, "sub")) },
		"WithNameLock":    func() error { return WithNameLock(dir, "api", nothing) },
		"WithRoomLock":    func() error { return WithRoomLock(dir, nothing) },
		"TryWithNameLock": func() error { _, err := TryWithNameLock(dir, "api", nothing); return err },
		"WithMailboxLock": func() error {
			return WithMailboxLock(context.Background(), dir, "api", nothing)
		},
		"Bootstrap":     func() error { _, err := Bootstrap(base, "blue"); return err },
		"OpenLeaseLock": func() error { _, err := OpenLeaseLock(filepath.Join(base, "v2", "rooms", "blue")); return err },
	}
	before := tree(t, base)
	for name, write := range writes {
		leave := EnterView()
		if err := write(); !errors.Is(err, ErrViewWrite) {
			t.Errorf("%s in view mode: %v", name, err)
		}
		if err := leave(); !errors.Is(err, ErrViewWrite) {
			t.Errorf("%s: leaving the mode reported %v", name, err)
		}
	}
	if after := tree(t, base); after != before {
		t.Fatalf("a view wrote:\nbefore %s\nafter  %s", before, after)
	}
	if data, err := ReadFile(file); err != nil || string(data) != "before" {
		t.Fatalf("the file: %q, %v", data, err)
	}
	// Left, the mode is gone: the same writes go through.
	if err := WriteAtomic(file, []byte("after")); err != nil {
		t.Fatal(err)
	}
	if err := EnterView()(); err != nil {
		t.Fatalf("a view without writes: %v", err)
	}
}

// TryHold takes an exclusive flock, so a view never gets one: it answers
// false before opening the file, and the attempt is kept as evidence although
// the bool carries no error — leaving the mode reports it.
func TestTryHoldRefusesInViewMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "held")
	writer, err := WriteAtomicHeld(path, []byte("pending"))
	if err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	opened := false
	beforeHold = func(string) { opened = true }
	t.Cleanup(func() { beforeHold = func(string) {} })
	leave := EnterView()
	held, ok := TryHold(path)
	if held != nil {
		_ = held.Close()
	}
	err = leave()
	if ok || opened || !errors.Is(err, ErrViewWrite) {
		t.Fatalf("in view mode: held %v, opened %v, leaving reported %v", ok, opened, err)
	}
	held, ok = TryHold(path)
	if !ok {
		t.Fatal("outside view mode the free lock was not taken")
	}
	_ = held.Close()
}
