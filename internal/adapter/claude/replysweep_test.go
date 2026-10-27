package claude

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
)

// A reply or mod socket nobody listens on is a killed wrapper's and goes; a
// live one, a young one, and anything that is not one of them, stays.
func TestLaneSweepsDeadReplySockets(t *testing.T) {
	dir := buildtest.Dir(t)
	dead := filepath.Join(dir, "gone.old.reply.sock")
	deadMod := filepath.Join(dir, "gone.old.mod.sock")
	for _, path := range []string{dead, deadMod} {
		listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
		if err != nil {
			t.Fatal(err)
		}
		listener.SetUnlinkOnClose(false)
		_ = listener.Close()
	}
	live := filepath.Join(dir, "here.now.reply.sock")
	alive, err := net.Listen("unix", live)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = alive.Close() }()
	session := filepath.Join(dir, "gone.old.sock")
	other, err := net.ListenUnix("unix", &net.UnixAddr{Name: session, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	other.SetUnlinkOnClose(false)
	_ = other.Close()

	young := filepath.Join(dir, "starting.now.reply.sock")
	bound, err := net.ListenUnix("unix", &net.UnixAddr{Name: young, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	bound.SetUnlinkOnClose(false)
	_ = bound.Close()
	old := time.Now().Add(-time.Minute)
	for _, path := range []string{dead, deadMod, session} {
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}

	sweepReplies(dir)
	for _, path := range []string{dead, deadMod} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("a dead socket %s stayed: %v", path, err)
		}
	}
	// A young socket may be bound and not yet listening.
	for _, path := range []string{live, session, young} {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("%s went: %v", path, err)
		}
	}
}
