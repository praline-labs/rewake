package state

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// The room's copy is a link to the install where the link holds the bytes
// verify wants, with the install's own mode untouched; else a copy of the
// image, executable by its owner alone; and a copy verify refuses fails the
// call and leaves no name behind.
func TestPlaceExecutableLinksOrCopies(t *testing.T) {
	base := filepath.Join(t.TempDir(), "state")
	dir, err := Bootstrap(base, DefaultRoom)
	if err != nil {
		t.Fatal(err)
	}
	leaseRoom(t, dir)
	outside := t.TempDir()
	installed := filepath.Join(outside, "rewake")
	image := filepath.Join(outside, "image")
	if err := os.WriteFile(installed, []byte("installed"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(image, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	wants := func(content string) func(string) error {
		return func(path string) error {
			data, err := os.ReadFile(path)
			if err != nil || string(data) != content {
				return errors.New("other bytes")
			}
			return nil
		}
	}
	path := filepath.Join(dir, "rewake")

	if err := PlaceExecutable(path, installed, image, wants("installed")); err != nil {
		t.Fatal(err)
	}
	if !sameFile(t, path, installed) || mode(t, installed) != 0o755 {
		t.Fatalf("not a link to the install, or its mode changed: %o", mode(t, installed))
	}

	if err := PlaceExecutable(path, installed, image, wants("image")); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "image" || sameFile(t, path, installed) || mode(t, path) != 0o700 {
		t.Fatalf("the copy: %q, mode %o", data, mode(t, path))
	}

	if err := PlaceExecutable(path, installed, image, wants("neither")); err == nil {
		t.Fatal("a copy verify refused was placed")
	}
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".tmp-") {
			t.Errorf("left behind: %s", entry.Name())
		}
	}
	if data, _ := os.ReadFile(path); string(data) != "image" {
		t.Fatalf("a refused copy replaced the placed one: %q", data)
	}
}

func sameFile(t *testing.T, a, b string) bool {
	t.Helper()
	first, err := os.Stat(a)
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.Stat(b)
	if err != nil {
		t.Fatal(err)
	}
	return first.Sys().(*syscall.Stat_t).Ino == second.Sys().(*syscall.Stat_t).Ino
}

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}
