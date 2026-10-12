package state

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// bootstrapLevels are the directories Bootstrap makes, from the base down.
func bootstrapLevels(base string) []string {
	room := filepath.Join(base, "rooms", "default")
	return []string{
		base, filepath.Join(base, "rooms"), room,
		filepath.Join(room, sessionsDir), filepath.Join(room, inboxDir), filepath.Join(room, socketsDir),
	}
}

func emptyBase(t *testing.T) string {
	t.Helper()
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(parent, "base")
}

// A level somebody else could write, a symlink or a file in a level's place
// is refused as it is: never chmodded, never replaced, and nothing below it
// made.
func TestBootstrapRefusesUnsafeExistingComponents(t *testing.T) {
	for index := range bootstrapLevels("/") {
		for _, unsafe := range []string{"open", "symlink", "file"} {
			t.Run(filepath.Base(bootstrapLevels("/x")[index])+" "+unsafe, func(t *testing.T) {
				base := emptyBase(t)
				levels := bootstrapLevels(base)
				for _, level := range levels[:index] {
					if err := os.Mkdir(level, 0o700); err != nil {
						t.Fatal(err)
					}
				}
				level := levels[index]
				switch unsafe {
				case "open":
					if err := os.Mkdir(level, 0o700); err != nil {
						t.Fatal(err)
					}
					if err := os.Chmod(level, 0o755); err != nil {
						t.Fatal(err)
					}
				case "symlink":
					if err := os.Symlink(t.TempDir(), level); err != nil {
						t.Fatal(err)
					}
				case "file":
					if err := os.WriteFile(level, nil, 0o600); err != nil {
						t.Fatal(err)
					}
				}
				before, err := os.Lstat(level)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := Bootstrap(base, "default"); err == nil {
					t.Fatal("bootstrapped over an unsafe level")
				}
				after, err := os.Lstat(level)
				if err != nil || after.Mode() != before.Mode() {
					t.Fatalf("the level changed: %v → %v, %v", before.Mode(), after, err)
				}
				for _, below := range levels[index+1:] {
					if _, err := os.Lstat(below); err == nil {
						t.Fatalf("%s made below the refused level", below)
					}
				}
			})
		}
	}
}

// A level whose flush into its parent fails stops the bootstrap there, so a
// launch never claims a name in a room a crash could take away; the next
// call completes what is missing.
func TestBootstrapSyncFailureStopsBeforeClaim(t *testing.T) {
	for index := range bootstrapLevels("/") {
		t.Run(filepath.Base(bootstrapLevels("/x")[index]), func(t *testing.T) {
			base := emptyBase(t)
			levels := bootstrapLevels(base)
			before := syncLevel
			t.Cleanup(func() { syncLevel = before })
			syncLevel = func(parent string) error {
				if parent == filepath.Dir(levels[index]) {
					return &os.PathError{Op: "fsync", Path: parent, Err: syscall.EIO}
				}
				return before(parent)
			}
			if _, err := Bootstrap(base, "default"); err == nil || !strings.Contains(err.Error(), "could not flush") {
				t.Fatalf("a failed flush of %s: %v", levels[index], err)
			}
			for _, below := range levels[index+1:] {
				if _, err := os.Lstat(below); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("%s made after the failed flush", below)
				}
			}
			syncLevel = before
			room, err := Bootstrap(base, "default")
			if err != nil || room != levels[2] {
				t.Fatalf("the next bootstrap: %q, %v", room, err)
			}
			for _, level := range levels {
				if err := Verify(level); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
