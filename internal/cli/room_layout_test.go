package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/state"
)

func TestRoomNamesCannotCollideWithLegacyMailboxes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	legacy := filepath.Join(root, "inbox", "sessions")
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(state.DirEnv, root)
	path := filepath.Join(legacy, "1789590000000000000-aabbccddeeff.json")
	raw := `{"id":"1789590000000000000-aabbccddeeff","from":"old","to":"sessions","text":"keep this work"}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, room := range []string{"inbox", "sessions", "sock"} {
		t.Setenv(state.RoomEnv, room)
		code, _, errOut := run("list", "--json")
		if code != 0 {
			t.Fatalf("list: %d %s", code, errOut)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != raw {
			t.Fatalf("room %s changed legacy queue: %v %q", room, err, got)
		}
		dir, err := state.Dir()
		if err != nil {
			t.Fatal(err)
		}
		if dir != filepath.Join(root, "rooms", room) {
			t.Errorf("ambiguous room directory %s", dir)
		}
	}
}
