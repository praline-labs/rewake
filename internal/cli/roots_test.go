package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/registry/registrytest"
	"github.com/praline-labs/rewake/internal/host/worktree"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// snapshot is every path below dir with its kind, mode, size, modification
// time and content: a write, a removal or a temporary file made and removed
// — which moves its directory's time — all change it. Access times are left
// out: reading is no mutation.
func snapshot(t *testing.T, dir string) string {
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
		line := fmt.Sprintf("%s %s %d %d", strings.TrimPrefix(path, dir), info.Mode(), info.Size(), info.ModTime().UnixNano())
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			line += " " + hex.EncodeToString(sum[:8])
		}
		lines = append(lines, line)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(lines, "\n")
}

// entries are the names in a directory, sorted.
func entries(t *testing.T, dir string) []string {
	t.Helper()
	found, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range found {
		names = append(names, entry.Name())
	}
	return names
}

// TestTwoRoomsUseOnlyV2 holds the root (docs/rules/state.md#the-roots):
// two rooms launched and written in leave one level in the base, v2, its
// rooms beside each other below it, and each room's mail in its own tree.
func TestTwoRoomsUseOnlyV2(t *testing.T) {
	t.Setenv(state.RoomEnv, "red")
	red := liveSession(t, "api")
	base := os.Getenv(state.DirEnv)
	otherRun(t, red, "web")
	t.Setenv(state.RoomEnv, "blue")
	blue := launched(t, base, "blue")
	self, err := registry.Load(red, "api")
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Publish(blue, self); err != nil {
		t.Fatal(err)
	}
	otherRun(t, blue, "web")
	t.Setenv(state.SessionEnv, "api")
	for room, text := range map[string]string{"red": "to red", "blue": "to blue"} {
		t.Setenv(state.RoomEnv, room)
		if code, out, errOut := run("send", "web", text, "--notify", "--wait", "0"); code != ExitOK && code != ExitPending {
			t.Fatalf("send in %s: %d %s %s", room, code, out, errOut)
		}
		run("list")
	}
	if got := entries(t, base); !slices.Equal(got, []string{"v2"}) {
		t.Fatalf("the base holds %v", got)
	}
	if got := entries(t, filepath.Join(base, "v2")); !slices.Equal(got, []string{"rooms"}) {
		t.Fatalf("the root holds %v", got)
	}
	if got := entries(t, filepath.Join(base, "v2", "rooms")); !slices.Equal(got, []string{"blue", "red"}) {
		t.Fatalf("the rooms are %v", got)
	}
	for dir, text := range map[string]string{red: "to red", blue: "to blue"} {
		files, _ := filepath.Glob(filepath.Join(state.InboxPath(dir, "web"), "*.json"))
		if len(files) != 1 {
			t.Fatalf("%s: %v", dir, files)
		}
		if data, _ := os.ReadFile(files[0]); !strings.Contains(string(data), text) {
			t.Fatalf("%s holds %s", dir, data)
		}
	}
}

// legacyTree is a 1.x room beside the root, as 1.x left it: its records,
// mail and lock in the base itself.
func legacyTree(t *testing.T, base string) string {
	t.Helper()
	room := filepath.Join(base, "rooms", "default")
	for _, sub := range []string{"sessions", "inbox/api"} {
		if err := os.MkdirAll(filepath.Join(room, sub), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		"sessions/api.json": `{"name":"api","pid":1,"pidStart":1}`,
		"inbox/api/1789590000000000000-aabbccddeeff.json": `{"id":"1789590000000000000-aabbccddeeff","from":"old","to":"api","text":"keep this work"}`,
		".launch.lock": "",
	} {
		if err := os.WriteFile(filepath.Join(room, path), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(base, "rooms")
}

// TestOrdinaryCommandsNeverTouchLegacyTree holds that 2.0 reads and writes
// only its root: with a 1.x room of the same name in the base, a launch's
// lease, change forms and views leave the 1.x tree byte for byte, and the
// sessions they show are 2.0's.
func TestOrdinaryCommandsNeverTouchLegacyTree(t *testing.T) {
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(parent, "state")
	if err := os.Mkdir(base, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := legacyTree(t, base)
	before := snapshot(t, legacy)
	t.Setenv(state.DirEnv, base)
	t.Setenv(state.RoomEnv, "")
	t.Setenv(state.SessionEnv, "")
	t.Setenv(epochEnv, "")
	if code, out, _ := run("list"); code != ExitOK || !strings.Contains(out, "No sessions are running") {
		t.Fatalf("list before a launch: %d %q", code, out)
	}
	dir := launched(t, base, state.DefaultRoom)
	web := otherRun(t, dir, "web")
	for _, argv := range [][]string{
		{"send", "web", "hello", "--wait", "0"},
		{"list"},
		{"list", "--json"},
		{"whoami"},
		{"guide"},
	} {
		run(argv...)
	}
	t.Setenv(state.SessionEnv, "web")
	t.Setenv(epochEnv, web.Epoch())
	for _, argv := range [][]string{{"inbox"}, {"inbox", "--owed"}, {"pending", "the review"}} {
		run(argv...)
	}
	if code, out, _ := run("list"); code != ExitOK || strings.Contains(out, "api") {
		t.Fatalf("list shows 1.x's session: %q", out)
	}
	if after := snapshot(t, legacy); after != before {
		t.Fatalf("the 1.x tree changed:\nbefore\n%s\nafter\n%s", before, after)
	}
}

// A checkout's owner is looked for in the root it registered in, checked: a
// 2.0 room of another base is asked, a 1.x room — whatever runs there — is
// skipped, never read two levels up as a root.
func TestWorktreeOwnerRootIsChecked(t *testing.T) {
	owned := liveSession(t, "api")
	self, err := registry.Load(owned, "api")
	if err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(t.TempDir(), "rooms", "default")
	registrytest.Legacy(t, legacy, self)
	t.Setenv(state.DirEnv, filepath.Join(t.TempDir(), "elsewhere"))
	record := func(dir string) worktree.Record {
		return worktree.Record{Path: t.TempDir(), Session: &worktree.Owner{Name: "api", Room: "default", Epoch: self.Epoch(), Dir: dir}}
	}
	if found, err := worktree.RunningIn(record(owned)); err != nil || !slices.Equal(found, []string{"api in room default"}) {
		t.Fatalf("the owner in a 2.0 room of another base: %v, %v", found, err)
	}
	if found, err := worktree.RunningIn(record(legacy)); err != nil || len(found) != 0 {
		t.Fatalf("the owner in a 1.x room: %v, %v", found, err)
	}
}
