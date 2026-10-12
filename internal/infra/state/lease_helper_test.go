package state

import (
	"testing"
	"time"
)

// leasedDir is a directory the test holds a room's lease for, as a session
// holds its room's: its build.lock taken shared and authorized until the test
// ends. The primitives' own tests write there; a write anywhere else is one
// no lease covers.
func leasedDir(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	leaseRoom(t, dir)
	return dir
}

// leaseRoom holds the lease of the room at dir until the test ends.
func leaseRoom(t testing.TB, dir string) {
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
}
