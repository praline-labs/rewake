package claude

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// fakeClaude puts a claude on PATH answering auth status as given.
func fakeClaude(t *testing.T, answer string, exit int) string {
	t.Helper()
	bin := t.TempDir()
	out := filepath.Join(bin, "env")
	script := "#!/bin/sh\nprintf '%s\\n' \"$CLAUDE_CONFIG_DIR $*\" > " + out + "\nprintf '%s\\n' '" + answer + "'\nexit " + strconv.Itoa(exit) + "\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return out
}

// A worker is refused while the folder has no login, naming the command the
// person runs; a login, a wrapper, or a status that says nothing lets it go.
func TestAWorkerWaitsForItsOwnLogin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := profileDir(home)
	seen := fakeClaude(t, `{"loggedIn": false, "authMethod": "none"}`, 1)
	err := New().(claudeHarness).WorkerRefusal("")
	if err == nil || !strings.Contains(err.Error(), configDirEnv+"="+dir+" claude auth login") {
		t.Fatalf("not logged in: %v", err)
	}
	if asked, _ := os.ReadFile(seen); strings.TrimSpace(string(asked)) != dir+" auth status --json" {
		t.Fatalf("asked %q", asked)
	}
	if err := New().(claudeHarness).WorkerRefusal("my-claude"); err != nil {
		t.Fatalf("under a wrapper: %v", err)
	}
	fakeClaude(t, `{"loggedIn": true}`, 0)
	if err := New().(claudeHarness).WorkerRefusal(""); err != nil {
		t.Fatalf("logged in: %v", err)
	}
	fakeClaude(t, `unknown command`, 1)
	if err := New().(claudeHarness).WorkerRefusal(""); err != nil {
		t.Fatalf("a status that says nothing: %v", err)
	}
}
