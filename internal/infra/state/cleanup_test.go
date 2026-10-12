package state

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

const testTreeID = "12.34.0123456789abcdef"

// legacyBase is a base with a 2.0 room the test leases and a 1.x tree beside
// it: rooms/old with its directories.
func legacyBase(t *testing.T) (string, *LeaseLock) {
	t.Helper()
	base, dir := bootstrapped(t)
	lock := heldLease(t, dir)
	for _, sub := range []string{sessionsDir, inboxDir, socketsDir} {
		if err := os.MkdirAll(filepath.Join(base, legacyRooms, "old", sub), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return base, lock
}

func cleanupOf(t *testing.T, lock *LeaseLock) *Cleanup {
	t.Helper()
	cleanup, err := AuthorizeCleanup(lock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup.Close)
	return cleanup
}

// A room's lease covers its room and nothing beside it: the trees of 1.x,
// the names a clearer gives them and the clear lock lie in no room, so the
// lease renames, removes and locks none of them, each refusal counted.
func TestARoomLeaseCannotClearTheTreesOf1x(t *testing.T) {
	base, _ := legacyBase(t)
	rooms := filepath.Join(base, legacyRooms)
	before := NoLeaseRefusals()
	if err := Rename(rooms, filepath.Join(base, candidatePrefix+testTreeID)); !errors.Is(err, ErrNoLease) {
		t.Fatalf("a room's lease set rooms/ aside: %v", err)
	}
	if err := RemoveAll(filepath.Join(rooms, "old")); !errors.Is(err, ErrNoLease) {
		t.Fatalf("a room's lease removed a room of 1.x: %v", err)
	}
	if _, err := OpenFile(filepath.Join(base, rootName, clearLockName), os.O_CREATE|os.O_RDWR, 0o600); !errors.Is(err, ErrNoLease) {
		t.Fatalf("a room's lease opened the clear lock to take it: %v", err)
	}
	if _, err := OpenFile(filepath.Join(rooms, "old", legacyLaunch), os.O_CREATE|os.O_RDWR, 0o600); !errors.Is(err, ErrNoLease) {
		t.Fatalf("a room's lease opened a 1.x launch lock to take it: %v", err)
	}
	if NoLeaseRefusals()-before != 4 {
		t.Fatalf("%d refusals counted, want 4", NoLeaseRefusals()-before)
	}
	if _, err := os.Lstat(filepath.Join(rooms, "old")); err != nil {
		t.Fatalf("a refused act changed the tree: %v", err)
	}
}

// The cleanup covers the trees of 1.x of its own base, the clear lock and the
// counts it records, spelled clean: a room of 2.0, any other name in the base,
// another base's tree and a name without a clearer's id are refused.
func TestTheCleanupCoversOnlyTheTreesOf1xOfItsBase(t *testing.T) {
	base, lock := legacyBase(t)
	cleanup := cleanupOf(t, lock)
	other, _ := bootstrapped(t)
	for _, path := range []string{
		filepath.Join(base, legacyRooms, "old"),
		filepath.Join(base, candidatePrefix+testTreeID, "old"),
		filepath.Join(base, validatedPrefix+testTreeID),
		filepath.Join(base, rootName, clearLockName),
		filepath.Join(base, rootName, summaryPrefix+testTreeID+summarySuffix),
	} {
		if !cleanup.covers(path) {
			t.Errorf("the cleanup does not cover %s", path)
		}
	}
	for _, path := range []string{
		base,
		filepath.Join(base, rootName),
		filepath.Join(base, rootName, "rooms", DefaultRoom, "build"),
		filepath.Join(base, "unrelated"),
		filepath.Join(base, candidatePrefix+"1.2"),
		filepath.Join(base, validatedPrefix+"../x"),
		filepath.Join(base, legacyRooms) + "/old/../../unrelated",
		filepath.Join(other, legacyRooms, "old"),
		filepath.Join(base, rootName, summaryPrefix+"x"+summarySuffix),
	} {
		if cleanup.covers(path) {
			t.Errorf("the cleanup covers %s", path)
		}
	}
	if _, err := cleanup.LockRoom(filepath.Join(base, rootName, "rooms", DefaultRoom), 0); err == nil {
		t.Fatal("the cleanup took the launch lock of a room of 2.0")
	}
	if err := cleanup.WriteSummary("x", nil); err == nil {
		t.Fatal("the cleanup recorded counts under a name with no id")
	}
}

// The capability lives no longer than the lease it was taken under, nor past
// its Close, and a view refuses first.
func TestTheCleanupDiesWithItsLease(t *testing.T) {
	base, lock := legacyBase(t)
	cleanup := cleanupOf(t, lock)

	leave := EnterView()
	_, err := cleanup.LockClear(0)
	_ = leave()
	if !errors.Is(err, ErrViewWrite) {
		t.Fatalf("a view took the clear lock: %v", err)
	}

	lock.Close()
	before := NoLeaseRefusals()
	if _, err := cleanup.LockClear(0); !errors.Is(err, ErrNoLease) {
		t.Fatalf("the clear lock taken after the lease went: %v", err)
	}
	if err := cleanup.SetAside(testTreeID); !errors.Is(err, ErrNoLease) {
		t.Fatalf("rooms/ set aside after the lease went: %v", err)
	}
	if NoLeaseRefusals()-before != 2 {
		t.Fatalf("%d refusals counted, want 2", NoLeaseRefusals()-before)
	}
	if _, err := AuthorizeCleanup(lock); !errors.Is(err, ErrNoLease) {
		t.Fatalf("a cleanup given for a closed lease: %v", err)
	}

	again := cleanupOf(t, heldLease(t, filepath.Join(base, rootName, "rooms", DefaultRoom)))
	again.Close()
	if _, err := again.LockClear(0); !errors.Is(err, ErrNoLease) {
		t.Fatalf("the clear lock taken after the cleanup's close: %v", err)
	}
}

// A transition never replaces: a candidate's name taken refuses setting aside,
// and a rooms/ that appeared refuses the restore, the candidate kept. Each
// flushes the base, and a failed flush fails the transition after the rename.
func TestATreeTransitionNeverReplaces(t *testing.T) {
	base, lock := legacyBase(t)
	cleanup := cleanupOf(t, lock)
	if err := os.Mkdir(cleanup.Candidate(testTreeID), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := cleanup.SetAside(testTreeID); !errors.Is(err, ErrTreeExists) {
		t.Fatalf("setting aside onto a taken name: %v", err)
	}
	if err := os.Remove(cleanup.Candidate(testTreeID)); err != nil {
		t.Fatal(err)
	}
	if err := cleanup.SetAside(testTreeID); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(cleanup.Rooms(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := cleanup.Restore(testTreeID); !errors.Is(err, ErrTreeExists) {
		t.Fatalf("a restore over a recreated rooms/: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(cleanup.Candidate(testTreeID), "old")); err != nil {
		t.Fatalf("a refused restore lost the candidate: %v", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(base, legacyRooms)); len(entries) != 0 {
		t.Fatalf("a refused restore wrote into rooms/: %v", entries)
	}

	defer func() { Fault = nil }()
	Fault = func(op, _ string) error {
		if op == OpSync {
			return syscall.EIO
		}
		return nil
	}
	if err := cleanup.MarkValidated(testTreeID); !errors.Is(err, syscall.EIO) {
		t.Fatalf("a transition whose flush failed: %v", err)
	}
}

// A room's launch lock taken before its tree was set aside is the file at the
// renamed path, which the clearer checks rather than opening it again; a file
// put in its place is told apart.
func TestARoomLockIsCheckedAtItsRenamedPath(t *testing.T) {
	_, lock := legacyBase(t)
	cleanup := cleanupOf(t, lock)
	held, err := cleanup.LockRoom(filepath.Join(cleanup.Rooms(), "old"), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	if _, err := cleanup.LockRoom(filepath.Join(cleanup.Rooms(), "old"), 20*time.Millisecond); !errors.Is(err, ErrLegacyBusy) {
		t.Fatalf("a held launch lock taken again: %v", err)
	}
	if err := cleanup.SetAside(testTreeID); err != nil {
		t.Fatal(err)
	}
	renamed := filepath.Join(cleanup.Candidate(testTreeID), "old", legacyLaunch)
	if same, err := held.At(renamed); err != nil || !same {
		t.Fatalf("the held lock is not the file at its renamed path: %v, %v", same, err)
	}
	if err := os.Remove(renamed); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(renamed, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if same, _ := held.At(renamed); same {
		t.Fatal("a replaced launch lock passed for the held one")
	}
}

// The clear lock waits its bound and no more; a probe of a lock never holds
// it, and a held one is not free.
func TestTheClearLockAndProbesKeepTheirBounds(t *testing.T) {
	base, lock := legacyBase(t)
	cleanup := cleanupOf(t, lock)
	first, err := cleanup.LockClear(0)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if _, err := cleanup.LockClear(50 * time.Millisecond); !errors.Is(err, ErrClearBusy) {
		t.Fatalf("a held clear lock taken: %v", err)
	}
	if waited := time.Since(started); waited > time.Second {
		t.Fatalf("the clear lock waited %s past its bound", waited)
	}
	first.Close()

	name := filepath.Join(base, legacyRooms, "old", sessionsDir, ".web.lock")
	if free, err := cleanup.TryLock(name); err != nil || !free {
		t.Fatalf("a missing name lock: %v, %v", free, err)
	}
	if _, err := os.Lstat(name); !os.IsNotExist(err) {
		t.Fatalf("a probe created the lock file: %v", err)
	}
	file, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	if free, err := cleanup.TryLock(name); err != nil || !free {
		t.Fatalf("a free name lock: %v, %v", free, err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	if free, err := cleanup.TryLock(name); err != nil || free {
		t.Fatalf("a held name lock: %v, %v", free, err)
	}
}

// Deleting a validated tree removes a link in it, never what it names.
func TestDeletingATreeNeverFollowsALink(t *testing.T) {
	base, lock := legacyBase(t)
	cleanup := cleanupOf(t, lock)
	outside := filepath.Join(t.TempDir(), "kept")
	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(base, legacyRooms, "old", "link")); err != nil {
		t.Fatal(err)
	}
	if err := cleanup.SetAside(testTreeID); err != nil {
		t.Fatal(err)
	}
	if err := cleanup.MarkValidated(testTreeID); err != nil {
		t.Fatal(err)
	}
	if err := cleanup.Delete(testTreeID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(cleanup.Validated(testTreeID)); !os.IsNotExist(err) {
		t.Fatalf("the tree stayed: %v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("a deletion followed a link out of the base: %v", err)
	}
}
