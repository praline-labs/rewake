package state

import (
	"errors"
	"net"
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

// A claim outside its own base is no proof the path is outside every state:
// a path in a room of another base, in its sock directory, in that base or
// its root beside the rooms, reached through a link into it, or in a room of
// 1.x, belongs to a state, and only the lease of the room it lies in binds,
// clears or unlinks it. The claiming room's lease writes nothing there, and
// refuses counted.
func TestASocketClaimNeverWritesAnotherBase(t *testing.T) {
	_, own := bootstrapped(t)
	heldLease(t, own)
	foreignBase, foreign := bootstrapped(t)
	outside, err := os.MkdirTemp("", "claim")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(outside) })
	link := filepath.Join(outside, "link")
	if err := os.Symlink(foreign, link); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(outside, "old", "rooms", DefaultRoom)
	for _, sub := range []string{sessionsDir, inboxDir, socketsDir} {
		if err := os.MkdirAll(filepath.Join(legacy, sub), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	places := map[string]string{
		"a room of another base":             filepath.Join(foreign, "reply.sock"),
		"its sock directory":                 filepath.Join(foreign, socketsDir, "reply.sock"),
		"the root of another base":           filepath.Join(foreignBase, rootName, "reply.sock"),
		"another base beside its root":       filepath.Join(foreignBase, "reply.sock"),
		"a link into a room of another base": filepath.Join(link, "reply.sock"),
		"a room of 1.x":                      filepath.Join(legacy, socketsDir, "reply.sock"),
	}
	for name, path := range places {
		t.Run(name, func(t *testing.T) {
			claim, err := ClaimSocket(own, path)
			if err != nil {
				t.Fatal(err)
			}
			before := NoLeaseRefusals()
			listener, err := claim.Listen(path)
			if listener != nil {
				_ = listener.Close()
			}
			if !errors.Is(err, ErrNoLease) {
				t.Fatalf("the own room's claim bound an unleased room in another base: %v", err)
			}
			if _, err := os.Lstat(path); !os.IsNotExist(err) {
				t.Fatalf("a refused bind left a file: %v", err)
			}
			deadSocket(t, path)
			if err := claim.Discard(path); !errors.Is(err, ErrNoLease) {
				t.Fatalf("the own room's claim cleared a socket in another base: %v", err)
			}
			if _, err := os.Lstat(path); err != nil {
				t.Fatalf("a refused clearing removed the socket: %v", err)
			}
			if NoLeaseRefusals()-before != 2 {
				t.Fatalf("%d refusals counted, want 2", NoLeaseRefusals()-before)
			}
			_ = os.Remove(path)
		})
	}

	// With that room's own lease the path is that room's to write, and its
	// close-time unlink goes with that lease, whatever the claiming room holds.
	path := filepath.Join(foreign, "reply.sock")
	claim, err := ClaimSocket(own, path)
	if err != nil {
		t.Fatal(err)
	}
	theirs := heldLease(t, foreign)
	listener, err := claim.Listen(path)
	if err != nil {
		t.Fatalf("binding under the lease of the room the path lies in: %v", err)
	}
	theirs.Close()
	if err := listener.Close(); !errors.Is(err, ErrNoLease) {
		t.Fatalf("the close-time unlink in another base without its lease: %v", err)
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("a refused unlink removed the socket: %v", err)
	}
}

// deadSocket leaves a socket at path, as a run that died bound would.
func deadSocket(t *testing.T, path string) {
	t.Helper()
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	listener.SetUnlinkOnClose(false)
	_ = listener.Close()
}
