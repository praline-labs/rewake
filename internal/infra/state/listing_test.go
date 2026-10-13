package state

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// A strict listing takes a directory for absent only when it is missing for
// real: a link at either level, dangling or to a directory elsewhere, and a
// file where a directory belongs are refused, never read as nothing there or
// as another tree.
func TestReadDirBelowTakesOnlyAMissingDirectoryForNone(t *testing.T) {
	setup := func(t *testing.T) (base, elsewhere string) {
		t.Helper()
		top := t.TempDir()
		base, elsewhere = filepath.Join(top, "room"), filepath.Join(top, "elsewhere")
		for _, dir := range []string{filepath.Join(base, "sessions"), elsewhere} {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(base, "sessions", "web.json"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		return base, elsewhere
	}
	relink := func(t *testing.T, path, target string) {
		t.Helper()
		if err := os.RemoveAll(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("a directory of its own", func(t *testing.T) {
		base, _ := setup(t)
		entries, err := ReadDirBelow(base, "sessions")
		if err != nil || len(entries) != 1 || entries[0].Name() != "web.json" {
			t.Fatalf("listed %v, %v", entries, err)
		}
	})
	missing := map[string]func(t *testing.T, base string){
		"the directory missing": func(t *testing.T, base string) { _ = os.RemoveAll(filepath.Join(base, "sessions")) },
		"its parent missing":    func(t *testing.T, base string) { _ = os.RemoveAll(base) },
	}
	for name, change := range missing {
		t.Run(name, func(t *testing.T) {
			base, _ := setup(t)
			change(t, base)
			if entries, err := ReadDirBelow(base, "sessions"); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("listed %v, %v; want absent", entries, err)
			}
		})
	}
	refused := map[string]func(t *testing.T, base, elsewhere string){
		"the directory a dangling link": func(t *testing.T, base, _ string) {
			relink(t, filepath.Join(base, "sessions"), filepath.Join(base, "gone"))
		},
		"the directory a link to a directory": func(t *testing.T, base, elsewhere string) {
			relink(t, filepath.Join(base, "sessions"), elsewhere)
		},
		"the directory a file": func(t *testing.T, base, _ string) {
			_ = os.RemoveAll(filepath.Join(base, "sessions"))
			if err := os.WriteFile(filepath.Join(base, "sessions"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"its parent a dangling link": func(t *testing.T, base, _ string) {
			relink(t, base, base+"-gone")
		},
		"its parent a link to a directory": func(t *testing.T, base, elsewhere string) {
			if err := os.Mkdir(filepath.Join(elsewhere, "sessions"), 0o700); err != nil {
				t.Fatal(err)
			}
			relink(t, base, elsewhere)
		},
	}
	for name, change := range refused {
		t.Run(name, func(t *testing.T) {
			base, elsewhere := setup(t)
			change(t, base, elsewhere)
			entries, err := ReadDirBelow(base, "sessions")
			if err == nil || errors.Is(err, os.ErrNotExist) {
				t.Fatalf("listed %v, %v; want a refusal", entries, err)
			}
		})
	}
}

// The directory is opened inside the parent that was checked: a parent
// swapped for a dangling link once it is open still lists what it held,
// never nothing there.
func TestReadDirBelowListsInsideTheParentItOpened(t *testing.T) {
	base := filepath.Join(t.TempDir(), "room")
	if err := os.MkdirAll(filepath.Join(base, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "sessions", "web.json"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	previous := betweenOpens
	betweenOpens = func(string) {
		if err := os.Rename(base, base+"-moved"); err != nil {
			t.Error(err)
		}
		if err := os.Symlink(base+"-gone", base); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(func() { betweenOpens = previous })
	entries, err := ReadDirBelow(base, "sessions")
	if err != nil || len(entries) != 1 {
		t.Fatalf("listed %v, %v; want the record of the parent opened", entries, err)
	}
}

// The rooms of a root are none only when rooms is missing for real; a link
// there, dangling or not, is refused.
func TestAllRoomDirsTakesOnlyMissingRoomsForNone(t *testing.T) {
	root := t.TempDir()
	if dirs, err := AllRoomDirs(root); err != nil || len(dirs) != 0 {
		t.Fatalf("a root with no rooms lists %v, %v", dirs, err)
	}
	elsewhere := filepath.Join(root, "elsewhere")
	if err := os.MkdirAll(filepath.Join(elsewhere, DefaultRoom), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]string{"dangling": filepath.Join(root, "gone"), "to a directory": elsewhere} {
		t.Run("rooms a link "+name, func(t *testing.T) {
			rooms := filepath.Join(root, "rooms")
			_ = os.Remove(rooms)
			if err := os.Symlink(target, rooms); err != nil {
				t.Fatal(err)
			}
			if dirs, err := AllRoomDirs(root); err == nil {
				t.Fatalf("listed %v through a link", dirs)
			}
		})
	}
}
