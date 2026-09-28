package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
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
	if code, out, errOut := run("list"); code != 0 || strings.Contains(out, "claude") && !strings.Contains(out, "No sessions") {
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

func TestRoomsAndWorkerAreLaunchFlagsOnly(t *testing.T) {
	for _, args := range [][]string{{"--room", "red", "--general", "codex"}, {"--room=blue", "--main", "claude"}} {
		if _, err := parse(args); err != nil {
			t.Errorf("launch %q: %v", args, err)
		}
	}
	for _, command := range []string{"list", "send", "inbox", "whoami"} {
		if _, err := parse([]string{"--room", "red", command}); err == nil {
			t.Errorf("room override accepted by %s", command)
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
	otherRun(t, root, "legacy-only")
	code, out, errOut := run("list")
	if code != 0 || !strings.Contains(out, "No sessions") {
		t.Errorf("legacy state: %d %s %s", code, out, errOut)
	}
}

func TestAnOccupiedMainIsAUsageRefusal(t *testing.T) {
	t.Setenv(state.RoomEnv, state.DefaultRoom)
	dir := liveSession(t, "leader")
	markMain(t, dir, "leader")
	// The launch comes from a shell outside any session.
	outsideAnySession(t)
	code, _, errOut := run("--main", "--name", "other", "claude")
	if code != ExitUsage || !strings.Contains(errOut, "leader") || !strings.Contains(errOut, "without --main") {
		t.Errorf("refusal=%d %s", code, errOut)
	}
}
