package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolateAliases keeps the person's own alias file and working directory out
// of a launch line under test.
func isolateAliases(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
}

// --command is refused before anything is launched when it names nothing that
// can run, and the refusal names the value and the next action.
func TestACommandThatCannotRunIsRefused(t *testing.T) {
	isolateAliases(t)
	plain := filepath.Join(t.TempDir(), "not-executable")
	if err := os.WriteFile(plain, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ value, want string }{
		{"no-such-wrapper-anywhere", "on PATH"},
		{"/nonexistent/my-claude", "at that path"},
		{plain, "at that path"},
	} {
		code, _, errOut := run("--command", tc.value, "claude")
		if code != ExitUsage || !strings.Contains(errOut, "--command "+tc.value) || !strings.Contains(errOut, tc.want) ||
			!strings.Contains(errOut, "omit --command to start claude itself") {
			t.Errorf("%s: exit %d, stderr %q", tc.value, code, errOut)
		}
	}
	if code, _, errOut := run("--command=", aHarness(t)); code != ExitUsage || !strings.Contains(errOut, "--command needs a program") {
		t.Errorf("empty: exit %d, stderr %q", code, errOut)
	}
}

// Named twice on the line, the program is refused rather than the last one
// silently winning.
func TestACommandGivenTwiceIsRefused(t *testing.T) {
	isolateAliases(t)
	code, _, errOut := run("--command", "sh", "--name", "x", "--command=sh", "claude")
	if code != ExitUsage || !strings.Contains(errOut, "--command given twice") || !strings.Contains(errOut, "full help:") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}

// After the harness word, --command belongs to the harness: rewake does not
// read it there.
func TestACommandAfterTheHarnessIsTheHarnesss(t *testing.T) {
	isolateAliases(t)
	if err := singleProgram([]string{"claude", "--command", "a", "--command", "b"}); err != nil {
		t.Errorf("flags after the harness word were read as rewake's: %v", err)
	}
}

// The option is in the launch commands' table, with a value, so help and the
// alias expansion both know it.
func TestTheCommandOptionIsALaunchFlag(t *testing.T) {
	if !launchFlagTakesValue("command") {
		t.Fatal("--command is not a launch flag taking a value")
	}
	code, out, _ := run("claude", "--help")
	if code != ExitOK || !strings.Contains(out, "--command <program>") || !strings.Contains(out, "command = \"<program>\"") {
		t.Errorf("help does not show --command and the alias field:\n%s", out)
	}
}

// A path with a slash is fixed against the launch directory, so a harness part
// started elsewhere, in a directory of its own, runs the same file.
func TestARelativeCommandIsFixedAgainstTheLaunchDirectory(t *testing.T) {
	isolateAliases(t)
	launchDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(launchDir, "my-harness"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	call := Call{Command: findCommand(aHarness(t)), Flags: map[string]string{"command": "./my-harness"}}
	program, err := launchProgram(call)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(launchDir, "my-harness"); program != want {
		t.Errorf("program = %q, want %q", program, want)
	}
	call.Flags["command"] = "sh"
	if program, err := launchProgram(call); err != nil || program != "sh" {
		t.Errorf("a name on PATH became %q (%v); it is passed on as a name", program, err)
	}
}
