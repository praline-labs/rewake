package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/role"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

func TestMainIsALaunchFlag(t *testing.T) {
	result, err := parse([]string{"--main", "--name", "lead", "claude", "--main"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	part, err := chosenRole(result.Call)
	if err != nil || part.ID != role.Main.ID {
		t.Errorf("role = %v, %v; want main", part.ID, err)
	}
	// After the harness name it belongs to the harness.
	if len(result.Call.Raw) != 1 || result.Call.Raw[0] != "--main" {
		t.Errorf("raw = %q, want the harness's own --main kept", result.Call.Raw)
	}
}

func TestWithoutAFlagASessionIsAWorker(t *testing.T) {
	result, err := parse([]string{"claude"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if part, _ := chosenRole(result.Call); part.ID != role.Worker.ID {
		t.Errorf("role = %s, want worker", part.ID)
	}
}

func TestMainIsRefusedOnOtherCommands(t *testing.T) {
	code, _, errOut := run("--main", "list")
	if code != ExitUsage || !strings.Contains(errOut, "list does not take --main") {
		t.Errorf("exit = %d, stderr = %q; want a refusal", code, errOut)
	}
}

// markMain turns the test's session into the main one.
func markMain(t *testing.T, dir, name string) {
	t.Helper()
	session, err := registry.Load(dir, name)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	session.Role = role.Main.ID
	encoded, _ := json.MarshalIndent(session, "", "  ")
	if err := os.WriteFile(state.SessionPath(dir, name), encoded, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestListNamesTheMainSession(t *testing.T) {
	dir := liveSession(t, "lead")
	markMain(t, dir, "lead")
	_, out, _ := run("list")
	if !strings.Contains(out, "(main)") {
		t.Errorf("list = %q, want the main session marked", out)
	}
}

// The main session reads reports and reports nothing: reading records no wait,
// and the end of its turn sends nothing, even to a session that wrote to it.
func TestTheMainSessionReportsNothing(t *testing.T) {
	dir := liveSession(t, "api")
	markMain(t, dir, "api")
	web := otherRun(t, dir, "web")
	readFrom(t, dir, web)
	if waiting := inbox.Waiters(dir, "api", epochOf(t, dir, "api")); len(waiting) != 0 {
		t.Errorf("waiting = %v, want none for the main session", waiting)
	}

	// Even a wait recorded some other way — by an older rewake, say — is not
	// reported.
	waits := filepath.Join(state.AwaitingPath(dir, "api"), epochOf(t, dir, "api"))
	if err := os.MkdirAll(waits, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(waits, "web"), []byte(web.Epoch()+" 1 m1"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	run("turn-ended", turnPayload)
	if found := finishedFor(t, dir, "web"); len(found) != 0 {
		t.Errorf("web holds %v, want no report from the main session", found)
	}
}

func TestWriteIsALaunchRole(t *testing.T) {
	result, err := parse([]string{"--write", "codex"})
	if err != nil {
		t.Fatal(err)
	}
	part, err := chosenRole(result.Call)
	if err != nil || part.ID != "write" || !part.GitWrite || part.Silent {
		t.Fatalf("role=%+v err=%v", part, err)
	}
	code, _, errOut := run("--write", "list")
	if code != ExitUsage || !strings.Contains(errOut, "list does not take --write") {
		t.Errorf("write accepted on list: %d %s", code, errOut)
	}
	for _, args := range [][]string{{"guide"}, {"codex", "--help"}} {
		code, out, errOut := run(args...)
		if code != 0 || !strings.Contains(out, "--write") {
			t.Errorf("help lacks write: %d %s %s", code, out, errOut)
		}
	}
}

func TestAWriterCannotAlsoBeMain(t *testing.T) {
	result, err := parse([]string{"--write", "--main", "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := chosenRole(result.Call); err == nil {
		t.Fatal("conflicting roles were accepted")
	}
}
