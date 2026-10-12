package state

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Each room is a tree of its own under the root v2 of the base, as the
// bootstrap makes it and the resolver names it.
func TestRoomsHaveSeparateStateTrees(t *testing.T) {
	base := filepath.Join(t.TempDir(), "state")
	t.Setenv(DirEnv, base)
	for _, room := range []string{"red", "blue", ""} {
		t.Setenv(RoomEnv, room)
		want := room
		if want == "" {
			want = DefaultRoom
		}
		made, err := Bootstrap(base, want)
		if err != nil {
			t.Fatal(err)
		}
		dir, err := Dir()
		if err != nil {
			t.Fatal(err)
		}
		if dir != filepath.Join(base, "v2", "rooms", want) || dir != made {
			t.Errorf("room=%q dir=%q, made %q", room, dir, made)
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
	for _, room := range []string{"../outside", "/tmp/outside", "Upper", ".", "a/b"} {
		t.Setenv(RoomEnv, room)
		if _, err := Dir(); err == nil {
			t.Errorf("accepted room %q", room)
		}
	}
}

func TestRoomDirectoriesRefuseSymlinks(t *testing.T) {
	base := filepath.Join(t.TempDir(), "state")
	t.Setenv(DirEnv, base)
	if _, err := Bootstrap(base, DefaultRoom); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(base, "v2", "rooms", "red")); err != nil {
		t.Fatal(err)
	}
	t.Setenv(RoomEnv, "red")
	if _, err := Dir(); err == nil {
		t.Fatal("symlinked room accepted")
	}
}

// No resolver makes anything (docs/v2/stage3-state.md#the-roots): on an empty
// parent, on a base alone, and on a base whose root has no rooms, each names
// the path and leaves the tree as it found it. Only the bootstrap creates.
func TestResolversNeverCreate(t *testing.T) {
	for _, made := range []string{"", "base", "root"} {
		t.Run("made "+made, func(t *testing.T) {
			parent := t.TempDir()
			if err := os.Chmod(parent, 0o700); err != nil {
				t.Fatal(err)
			}
			base := filepath.Join(parent, "state")
			switch made {
			case "base":
				mustMkdir(t, base)
			case "root":
				mustMkdir(t, base)
				mustMkdir(t, filepath.Join(base, "v2"))
			}
			t.Setenv(DirEnv, base)
			t.Setenv(RoomEnv, "red")
			before := tree(t, parent)

			if got, err := Base(); err != nil || got != base {
				t.Errorf("Base = %q, %v", got, err)
			}
			root, err := Root()
			if err != nil || root != filepath.Join(base, "v2") {
				t.Errorf("Root = %q, %v", root, err)
			}
			room := filepath.Join(root, "rooms", "red")
			if got, err := Dir(); err != nil || got != room {
				t.Errorf("Dir = %q, %v", got, err)
			}
			if got, err := RoomDir(root, "blue"); err != nil || got != filepath.Join(root, "rooms", "blue") {
				t.Errorf("RoomDir = %q, %v", got, err)
			}
			if got, err := RootForRoom(room); err != nil || got != root {
				t.Errorf("RootForRoom = %q, %v", got, err)
			}
			if got, err := BaseForRoom(room); err != nil || got != base {
				t.Errorf("BaseForRoom = %q, %v", got, err)
			}
			if got, err := RoomPath(base, "red"); err != nil || got != room {
				t.Errorf("RoomPath = %q, %v", got, err)
			}
			if rooms, err := RoomDirs(root); err != nil || len(rooms) != 0 {
				t.Errorf("RoomDirs = %v, %v", rooms, err)
			}
			if after := tree(t, parent); after != before {
				t.Fatalf("a resolver made something:\nbefore %s\nafter  %s", before, after)
			}
		})
	}
}

// The inverse is checked, never counted: only <root>/rooms/<room> under a
// root named v2 has a root, and a 1.x room, a path of another shape or a
// relative one is refused — never handed on as the directory two levels up.
func TestRootInverseRejectsLegacyOrMalformedRoom(t *testing.T) {
	base := filepath.Join(t.TempDir(), "state")
	good := filepath.Join(base, "v2", "rooms", "red")
	if root, err := RootForRoom(good); err != nil || root != filepath.Join(base, "v2") {
		t.Fatalf("a 2.0 room: %q, %v", root, err)
	}
	for _, dir := range []string{
		filepath.Join(base, "rooms", "red"),                // 1.x's room
		filepath.Join(base, "v2", "red"),                   // no rooms level
		filepath.Join(base, "v3", "rooms", "red"),          // another root
		filepath.Join(base, "v2", "rooms", "Red"),          // not a room name
		filepath.Join(base, "v2", "rooms", "red") + "/",    // not clean
		filepath.Join(base, "v2", "rooms", "red", "inbox"), // below a room
		"v2/rooms/red", // relative
		filepath.Join(base, "v2", "rooms", "red", ".."),
	} {
		if root, err := RootForRoom(dir); err == nil {
			t.Errorf("RootForRoom(%q) = %q", dir, root)
		}
		if got, err := BaseForRoom(dir); err == nil {
			t.Errorf("BaseForRoom(%q) = %q", dir, got)
		}
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

// tree is every path below dir with its kind and mode.
func tree(t *testing.T, dir string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		lines = append(lines, strings.TrimPrefix(path, dir)+" "+info.Mode().String())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(lines, "; ")
}
