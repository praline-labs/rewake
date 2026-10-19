package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/registry/registrytest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

func TestCommandsCannotSeeAnotherRoom(t *testing.T) {
	t.Setenv("REWAKE_ROOM", "red")
	red := liveSession(t, "api")
	session, err := registry.Lookup(red, "api")
	if err != nil {
		t.Fatal(err)
	}
	rawUnread(t, red, "api", map[string]any{"text": "red secret", "toEpoch": session.Epoch()})
	t.Setenv("REWAKE_ROOM", "blue")
	launched(t, os.Getenv(state.DirEnv), "blue")
	if code, out, errOut := run("list"); code != 0 || strings.Contains(out, agentID) && !strings.Contains(out, "No sessions") {
		t.Errorf("foreign list: %d %s %s", code, out, errOut)
	}
	if code, _, _ := run("send", "api", "hello", "--wait", "0"); code != ExitUsage {
		t.Errorf("send crossed rooms: %d", code)
	}
	t.Setenv(state.SessionEnv, "api")
	if code, out, _ := run("inbox"); code == 0 || strings.Contains(out, "red secret") {
		t.Errorf("foreign inbox: %d %s", code, out)
	}
	t.Setenv("REWAKE_ROOM", "red")
	if code, out, _ := run("inbox"); code != 0 || !strings.Contains(out, "red secret") {
		t.Errorf("original inbox lost: %d %s", code, out)
	}
}

func TestSameNamesInDifferentRoomsKeepSeparateMail(t *testing.T) {
	t.Setenv("REWAKE_ROOM", "red")
	red := liveSession(t, "api")
	session, _ := registry.Lookup(red, "api")
	t.Setenv("REWAKE_ROOM", "blue")
	launched(t, os.Getenv(state.DirEnv), "blue")
	blue, err := state.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Publish(blue, session); err != nil {
		t.Fatalf("same name in another room: %v", err)
	}
	if code, _, _ := run("send", "api", "blue only", "--wait", "0"); code != ExitPending {
		t.Fatalf("blue send=%d", code)
	}
	if files, _ := filepath.Glob(filepath.Join(state.InboxPath(red, "api"), "*.json")); len(files) != 0 {
		t.Errorf("mail leaked to red: %v", files)
	}
	if files, _ := filepath.Glob(filepath.Join(state.InboxPath(blue, "api"), "*.json")); len(files) != 1 {
		t.Errorf("missing blue mail: %v", files)
	}
}

func TestRoomAndRoleAreVisibleInIdentity(t *testing.T) {
	t.Setenv("REWAKE_ROOM", "red")
	liveSession(t, "api")
	t.Setenv(state.SessionEnv, "api")
	for _, command := range []string{"list", "whoami"} {
		code, out, errOut := run(command, "--json")
		if code != 0 {
			t.Fatalf("%s: %s", command, errOut)
		}
		var model map[string]any
		if err := json.Unmarshal([]byte(out), &model); err != nil {
			t.Fatal(err)
		}
		if command == "list" {
			model = model["sessions"].([]any)[0].(map[string]any)
		}
		if model["room"] != "red" || model["role"] != "general" {
			t.Errorf("%s identity=%s", command, out)
		}
	}
}

func TestLegacyRootRecordsAreIgnored(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	t.Setenv(state.DirEnv, root)
	t.Setenv("REWAKE_ROOM", "")
	if err := os.MkdirAll(filepath.Join(root, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sessions", "bad.json"), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	registrytest.Legacy(t, root, otherSession(t, root, "legacy-only"))
	code, out, errOut := run("list")
	if code != 0 || !strings.Contains(out, "No sessions") {
		t.Errorf("legacy state: %d %s %s", code, out, errOut)
	}
}

// No command takes a room: every command's is its session's or its
// directory's.
func TestNoCommandTakesARoom(t *testing.T) {
	for _, command := range []string{"list", "send", "inbox", "whoami"} {
		if _, err := testTable().parse([]string{"--room", "red", command}); err == nil {
			t.Errorf("a room accepted by %s", command)
		}
	}
}

// A second main in one directory is refused, exit 1, naming the directory
// and the main that runs there.
func TestASecondMainInTheDirectoryIsRefused(t *testing.T) {
	launchDir := t.TempDir()
	t.Chdir(launchDir)
	room, resolved, err := state.RoomOf(launchDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(state.RoomEnv, room)
	dir := liveSession(t, "leader")
	markMain(t, dir, "leader")
	// The launch comes from a shell outside any session.
	outsideAnySession(t)
	t.Setenv(state.RoomEnv, "")
	code, _, errOut := run(agentID)
	if code != ExitFailed || !strings.Contains(errOut, resolved+" already has main leader") || !strings.Contains(errOut, "one main per directory") {
		t.Errorf("refusal=%d %s", code, errOut)
	}
}
