package state

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// Every write primitive writes the room whose lease this process holds, and
// refuses with ErrNoLease, changing nothing, once that lease is let go; the
// bootstrap's writes alone need none. Each primitive is run on a scene of its
// own, so one write does not undo the next one's ground.
func TestEveryMutationRequiresItsRoomLease(t *testing.T) {
	probeBase, probeDir := leasedBase(t)
	probe, _ := writePrimitives(t, probeBase, probeDir)
	names := make([]string, 0, len(probe))
	for name := range probe {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			base, dir := bootstrapped(t)
			heldLease(t, dir)
			writes, _ := writePrimitives(t, base, dir)
			if err := writes[name](); err != nil {
				t.Fatalf("under the lease: %v", err)
			}
			base, dir = bootstrapped(t)
			lock := heldLease(t, dir)
			writes, _ = writePrimitives(t, base, dir)
			lock.Close()
			before, refusals := tree(t, base), NoLeaseRefusals()
			err := writes[name]()
			if leaseFree[name] {
				if err != nil {
					t.Fatalf("a bootstrap write refused without a lease: %v", err)
				}
				return
			}
			if !errors.Is(err, ErrNoLease) {
				t.Fatalf("without the lease: %v", err)
			}
			if NoLeaseRefusals() == refusals {
				t.Fatal("the refusal was not counted")
			}
			if after := tree(t, base); after != before {
				t.Fatalf("a refused write changed the state:\nbefore %s\nafter  %s", before, after)
			}
		})
	}
	// TryHold answers a bool: without the lease it takes nothing.
	_, dir := bootstrapped(t)
	lock := heldLease(t, dir)
	path := filepath.Join(dir, "held")
	writer, err := WriteAtomicHeld(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	lock.Close()
	if held, ok := TryHold(path); ok {
		_ = held.Close()
		t.Fatal("TryHold took a lock without the lease")
	}
}

// In view mode a write refuses as a view's whether or not a lease is held: the
// view's refusal comes first (D3), so a view without a lease is never told it
// lacks one.
func TestViewWritePrecedesNoLease(t *testing.T) {
	base, dir := bootstrapped(t)
	lock := heldLease(t, dir)
	writes, _ := writePrimitives(t, base, dir)
	lock.Close()
	for name, write := range writes {
		leave := EnterView()
		err := write()
		_ = leave()
		if !errors.Is(err, ErrViewWrite) || errors.Is(err, ErrNoLease) {
			t.Errorf("%s in view mode without a lease: %v", name, err)
		}
	}
}

// A lease covers its own room only: a write of another room, of a room whose
// name the leased one's begins, or with one end of a link or a rename in
// another room, refuses. A room stays leased while any of its authorized
// locks is open, and not after the last closes.
func TestCrossRoomAndReleasedLeaseCannotWrite(t *testing.T) {
	base, blue := bootstrappedRoom(t, "", "blue")
	blueLock := heldLease(t, blue)
	writes, _ := writePrimitives(t, base, blue)
	blueFile := filepath.Join(blue, "file")
	blueLock.Close()
	_, dir := bootstrappedRoom(t, base, DefaultRoom)
	_, near := bootstrappedRoom(t, base, DefaultRoom+"x")
	first := heldLease(t, dir)
	for name, write := range writes {
		if err := write(); !leaseFree[name] && !errors.Is(err, ErrNoLease) {
			t.Errorf("%s of another room: %v", name, err)
		}
	}
	file := filepath.Join(dir, "file")
	if err := WriteAtomic(file, []byte("mine")); err != nil {
		t.Fatal(err)
	}
	crossing := map[string]error{
		"a room whose name begins the same": WriteAtomic(filepath.Join(near, "file"), nil),
		"a link out":                        Link(file, filepath.Join(near, "linked")),
		"a link in":                         Link(blueFile, filepath.Join(dir, "linked")),
		"a move out":                        Move(file, filepath.Join(near, "moved")),
		"a rename in":                       Rename(blueFile, filepath.Join(dir, "renamed")),
		"a rename out":                      Rename(file, filepath.Join(blue, "renamed")),
	}
	for name, err := range crossing {
		if !errors.Is(err, ErrNoLease) {
			t.Errorf("%s: %v", name, err)
		}
	}
	second := heldLease(t, dir)
	first.Close()
	if err := WriteAtomic(file, []byte("still")); err != nil {
		t.Fatalf("with one of two locks left: %v", err)
	}
	second.Close()
	if err := WriteAtomic(file, []byte("gone")); !errors.Is(err, ErrNoLease) {
		t.Fatalf("after the last lock closed: %v", err)
	}
}

// bootstrapped is a fresh base with its default room made and no lease held.
func bootstrapped(t *testing.T) (base, dir string) {
	t.Helper()
	return bootstrappedRoom(t, "", DefaultRoom)
}

// bootstrappedRoom makes room in base, a fresh one when base is empty.
func bootstrappedRoom(t *testing.T, base, room string) (string, string) {
	t.Helper()
	if base == "" {
		// A short directory of its own: the scene binds a socket, and a
		// path has a bound a test's name can pass.
		short, err := os.MkdirTemp("", "lg")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(short) })
		base = filepath.Join(short, "state")
	}
	dir, err := Bootstrap(base, room)
	if err != nil {
		t.Fatal(err)
	}
	return base, dir
}

// leasedBase is a fresh base and room the test holds the lease of to its end.
func leasedBase(t *testing.T) (string, string) {
	t.Helper()
	base, dir := bootstrapped(t)
	leaseRoom(t, dir)
	return base, dir
}

// heldLease takes the room's lease, shared and authorized, and gives the lock
// for the test to let go; whatever it does not, the test's end closes.
func heldLease(t *testing.T, dir string) *LeaseLock {
	t.Helper()
	lock, err := CreateLeaseLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Share(time.Second); err != nil {
		t.Fatal(err)
	}
	if err := Authorize(lock); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(lock.Close)
	return lock
}
