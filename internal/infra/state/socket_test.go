package state

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

// A socket of the state is private, removed when it closes, and removed
// once: a second close leaves the socket the next run of the name bound.
func TestASocketIsRemovedOnceByItsOwnClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "api.sock")
	first, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != 0o600 {
		t.Fatalf("the bound socket: %v, %v", info, err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("the closed socket is still there: %v", err)
	}
	next, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = next.Close() }()
	_ = first.Close()
	connection, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("a second close removed the next run's socket: %v", err)
	}
	_ = connection.Close()
}

// A close in view mode closes the listener and leaves its file: the removal
// is the write the mode refuses.
func TestAClosedSocketStaysWhenItsRemovalIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "api.sock")
	listener, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	leave := EnterView()
	_ = listener.Close()
	if err := leave(); err == nil {
		t.Error("the refused removal left no evidence")
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("a refused removal removed the socket: %v", err)
	}
	if _, err := net.Dial("unix", path); err == nil {
		t.Fatal("the listener still accepts after its close")
	}
}
