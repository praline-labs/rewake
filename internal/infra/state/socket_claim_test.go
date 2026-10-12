package state

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// A claim covers one socket path outside the room and three acts on it, and
// only while the room's lease holds: it binds there and its close removes the
// file; a dead socket there is cleared, anything else is not; another path
// is refused; a view refuses first; with the lease let go each act refuses,
// counted, and leaves the file; inside the state it widens nothing, a path in
// another room refusing as that room's write would.
func TestASocketClaimCoversOnePathWhileTheLeaseHolds(t *testing.T) {
	base, dir := bootstrapped(t)
	lock := heldLease(t, dir)
	outside, err := os.MkdirTemp("", "claim")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(outside) })
	path := filepath.Join(outside, "reply.sock")
	claim, err := ClaimSocket(dir, path)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := claim.Listen(path)
	if err != nil {
		t.Fatalf("binding the claimed path: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("the close left the claimed socket: %v", err)
	}

	other := filepath.Join(outside, "other.sock")
	if _, err := claim.Listen(other); err == nil {
		t.Fatal("a claim bound a path it does not name")
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := claim.Discard(path); err == nil {
		t.Fatal("a claim removed a file that is no socket")
	}
	_ = os.Remove(path)

	leave := EnterView()
	_, err = claim.Listen(path)
	_ = leave()
	if !errors.Is(err, ErrViewWrite) {
		t.Fatalf("a view bound the claimed path: %v", err)
	}

	held, err := claim.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	lock.Close()
	before := NoLeaseRefusals()
	if err := held.Close(); !errors.Is(err, ErrNoLease) {
		t.Fatalf("the close-time unlink without the lease: %v", err)
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("a refused unlink removed the socket: %v", err)
	}
	if err := claim.Discard(path); !errors.Is(err, ErrNoLease) {
		t.Fatalf("clearing the claimed path without the lease: %v", err)
	}
	if _, err := claim.Listen(path); !errors.Is(err, ErrNoLease) {
		t.Fatalf("binding the claimed path without the lease: %v", err)
	}
	if NoLeaseRefusals()-before != 3 {
		t.Fatalf("%d refusals counted, want 3", NoLeaseRefusals()-before)
	}

	heldLease(t, dir)
	_, near := bootstrappedRoom(t, base, DefaultRoom+"x")
	inside, err := ClaimSocket(dir, filepath.Join(near, "reply.sock"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inside.Listen(filepath.Join(near, "reply.sock")); !errors.Is(err, ErrNoLease) {
		t.Fatalf("a claim bound a path in another room: %v", err)
	}
	if _, err := ClaimSocket(outside, path); err == nil {
		t.Fatal("a directory that is no room claimed a socket")
	}
}
