package claude

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
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

// While the folder has no login, main is told to have the person log in in
// the worker's pane; a login, a wrapper, or a status that says nothing tells
// it nothing.
func TestMainIsToldTheWorkersHaveNoLogin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := profileDir(home)
	seen := fakeClaude(t, `{"loggedIn": false, "authMethod": "none"}`, 1)
	notice := New().(claudeHarness).WorkerNotice("")
	if !strings.Contains(notice, dir) || !strings.Contains(notice, "in this worker's pane with /login") {
		t.Fatalf("not logged in: %q", notice)
	}
	if asked, _ := os.ReadFile(seen); strings.TrimSpace(string(asked)) != dir+" auth status --json" {
		t.Fatalf("asked %q", asked)
	}
	if notice := New().(claudeHarness).WorkerNotice("my-claude"); notice != "" {
		t.Fatalf("under a wrapper: %q", notice)
	}
	fakeClaude(t, `{"loggedIn": true}`, 0)
	if notice := New().(claudeHarness).WorkerNotice(""); notice != "" {
		t.Fatalf("logged in: %q", notice)
	}
	fakeClaude(t, `unknown command`, 1)
	if notice := New().(claudeHarness).WorkerNotice(""); notice != "" {
		t.Fatalf("a status that says nothing: %q", notice)
	}
}

// The case review found: a claude on PATH whose child keeps the status's
// pipes open held the open past the deadline. The wait is bounded, and a
// status that does not answer says nothing.
func TestAStatusThatHangsIsBounded(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	child := filepath.Join(bin, "child")
	script := "#!/bin/sh\nsleep 60 &\necho $! > " + child + "\nwait\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Cleanup(func() {
		raw, _ := os.ReadFile(child)
		if pid, _ := strconv.Atoi(strings.TrimSpace(string(raw))); pid > 0 {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})
	defer func(wait time.Duration) { loginWait = wait }(loginWait)
	loginWait = 200 * time.Millisecond
	done := make(chan string, 1)
	go func() { done <- New().(claudeHarness).WorkerNotice("") }()
	select {
	case notice := <-done:
		if notice != "" {
			t.Errorf("a status that does not answer: %q", notice)
		}
	case <-time.After(loginWait + statusWaitDelay + 3*time.Second):
		t.Error("the status check waited past its bound")
	}
}
