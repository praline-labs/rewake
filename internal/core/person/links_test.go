package person

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/core/registry"
)

// A clean chain is the person only over inventories read whole: rooms or a
// room's registry put behind a link, dangling or to a directory elsewhere,
// is no scan that found no session, and the answer is unreadable. Missing
// for real, they hold no session, and the chain decides.
func TestPersonRefusesInventoriesBehindLinks(t *testing.T) {
	cases := []struct {
		level    string
		dangling bool
	}{{"rooms", true}, {"sessions", true}, {"rooms", false}, {"sessions", false}}
	for _, c := range cases {
		name := c.level + " a link to a directory"
		if c.dangling {
			name = c.level + " a dangling link"
		}
		t.Run(name, func(t *testing.T) {
			scan := scanOf(t, cleanChain(t), registry.Inventory)
			path := filepath.Join(scan.Root, "rooms")
			if c.level == "sessions" {
				path = filepath.Join(path, "default", "sessions")
			} else if err := os.RemoveAll(path); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(scan.Root, "missing-inventory")
			if !c.dangling {
				// Rooms lead to a tree with a room and no session in it; a
				// registry to an empty directory.
				target = filepath.Join(scan.Root, "elsewhere")
				mkdir(t, target)
				if c.level == "rooms" {
					mkdir(t, filepath.Join(target, "default"))
				}
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
			if got := verdict(scan.Classify()); got != "unreadable" {
				t.Fatalf("%s classified as %s", name, got)
			}
		})
	}
	for _, level := range []string{"rooms", "sessions"} {
		t.Run(level+" missing", func(t *testing.T) {
			scan := scanOf(t, cleanChain(t), registry.Inventory)
			path := filepath.Join(scan.Root, "rooms")
			if level == "sessions" {
				path = filepath.Join(path, "default", "sessions")
			}
			if err := os.RemoveAll(path); err != nil {
				t.Fatal(err)
			}
			if got := verdict(scan.Classify()); got != "person" {
				t.Fatalf("%s missing classified as %s", level, got)
			}
		})
	}
}

// cleanChain is process 100 under a shell of the user, under init, neither
// in a session.
func cleanChain(t *testing.T) string {
	t.Helper()
	return procTree(t,
		process{pid: 100, parent: 50, uid: user, start: 300, env: []string{}},
		process{pid: 50, parent: 1, uid: user, start: 200, env: []string{}},
		initProcess)
}
