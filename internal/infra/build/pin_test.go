package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A launch leaves the room its own copy of the launching build, with the
// directory the image was installed in beside it; a launch of the same build
// leaves the copy as it is; a launch that switches the room replaces it.
func TestLaunchKeepsTheRoomsOwnCopy(t *testing.T) {
	base := newBase(t)
	lease, err := Launch(base, "default")
	if err != nil {
		t.Fatal(err)
	}
	room := roomOf(base, "default")
	path := Pinned(room)
	own, _ := ID()
	if lease.Executable() != path || lease.Note() != "" {
		t.Fatalf("executable %q, note %q", lease.Executable(), lease.Note())
	}
	if got, err := FileID(path); err != nil || got != own {
		t.Fatalf("the copy: %q, %v", got, err)
	}
	self, _ := os.Executable()
	self, _ = filepath.EvalSymlinks(self)
	if Origin(path) != filepath.Dir(self) {
		t.Fatalf("origin %q, installed in %q", Origin(path), filepath.Dir(self))
	}
	before, _ := os.Stat(path)
	again, err := Launch(base, "default")
	if err != nil {
		t.Fatal(err)
	}
	again.Release()
	lease.Release()
	if after, _ := os.Stat(path); !os.SameFile(before, after) {
		t.Fatal("a launch of the room's build placed the copy again")
	}

	other := actAs(t, "another build")
	switched, err := Launch(base, "default")
	if err != nil {
		t.Fatal(err)
	}
	defer switched.Release()
	if got, err := FileID(path); err != nil || got != other {
		t.Fatalf("after a switch the copy is %q, %v", got, err)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o700 {
		t.Fatalf("a copied image's mode: %o", info.Mode().Perm())
	}
}

// A room of another build hands a call to its copy only when the copy is that
// build; a room of this build, one without a copy and one whose copy is other
// bytes hand nothing off.
func TestHandoffOnlyToACopyOfTheRoomsBuild(t *testing.T) {
	base := newBase(t)
	lease, err := Launch(base, "default")
	if err != nil {
		t.Fatal(err)
	}
	lease.Release()
	room := roomOf(base, "default")
	theirs, _ := ID()
	if _, _, ok := Handoff(room); ok {
		t.Fatal("a room of this build handed off")
	}
	actAs(t, "a newer build")
	path, recorded, ok := Handoff(room)
	if !ok || path != Pinned(room) || recorded != theirs {
		t.Fatalf("Handoff = %q, %q, %v", path, recorded, ok)
	}
	if line := HandoffLine(room, theirs); !strings.Contains(line, Short(theirs)) || !strings.Contains(line, "Restarting main") {
		t.Fatalf("the line: %q", line)
	}
	// Replaced, not written: the copy is a link to the running test binary.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("other bytes"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := Handoff(room); ok {
		t.Fatal("handed off to a copy of other bytes")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := Handoff(room); ok {
		t.Fatal("handed off to a missing copy")
	}
}

// Origin reads the file beside a room's copy only: an executable elsewhere
// was installed where it is.
func TestOriginOfACopyAndOfAnInstall(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "room", "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, OriginName), []byte("/opt/install/bin\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Origin(filepath.Join(bin, "rewake")); got != "/opt/install/bin" {
		t.Fatalf("a copy's origin: %q", got)
	}
	if got := Origin(filepath.Join(dir, "rewake")); got != dir {
		t.Fatalf("an install's origin: %q", got)
	}
}
