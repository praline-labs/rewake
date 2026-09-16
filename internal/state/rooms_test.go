package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoomsHaveSeparateStateTrees(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	t.Setenv(DirEnv, root)
	for _, room := range []string{"red", "blue", ""} {
		t.Setenv("REWAKE_ROOM", room)
		dir, err := Dir()
		if err != nil {
			t.Fatal(err)
		}
		want := room
		if want == "" {
			want = "default"
		}
		if dir != filepath.Join(root, "rooms", want) {
			t.Errorf("room=%q dir=%q", room, dir)
		}
		for _, sub := range []string{"sessions", "inbox", "sock"} {
			if _, err := os.Stat(filepath.Join(dir, sub)); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestInvalidRoomsCannotEscapeTheStateRoot(t *testing.T) {
	t.Setenv(DirEnv, filepath.Join(t.TempDir(), "state"))
	for _, room := range []string{"../outside", "/tmp/outside", "Upper", ".", "", "a/b"} {
		if room == "" {
			continue
		}
		t.Setenv("REWAKE_ROOM", room)
		if _, err := Dir(); err == nil {
			t.Errorf("accepted room %q", room)
		}
	}
}

func TestRoomDirectoriesRefuseSymlinks(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	t.Setenv(DirEnv, root)
	if err := os.MkdirAll(filepath.Join(root, "rooms"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "rooms", "red")); err != nil {
		t.Fatal(err)
	}
	t.Setenv(RoomEnv, "red")
	if _, err := Dir(); err == nil {
		t.Fatal("symlinked room accepted")
	}
}

func TestLongRoomAndSessionNamesKeepSocketsShort(t *testing.T) {
	room := "rrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrr"
	name := "nnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnn"
	dir := filepath.Join("/tmp/example-state", room)
	first := SocketPath(dir, name, "1234567890.12345678901234567890")
	second := SocketPath(dir, name, "1234567891.12345678901234567890")
	if len(first) > 103 || first == second {
		t.Errorf("socket paths=%q %q", first, second)
	}
}
