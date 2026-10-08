package harness

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The launch's version (docs/mail-bridge-version.md): a read answers one of
// three — a version, unknown once its cleanup is proven, or the check's
// failure — and a gate or a bound recorded for a version holds for it alone.

func TestAVersionReadAnswersOneOfThree(t *testing.T) {
	saved := versionBound
	t.Cleanup(func() { versionBound = saved })
	versionBound = 500 * time.Millisecond
	for _, tc := range []struct {
		body string
		want Version
	}{
		{`echo "codex-cli 0.159.0"`, Version{Value: "0.159.0"}},
		{`echo "2.1.284 (Claude Code)"`, Version{Value: "2.1.284"}},
		{`echo "codex-cli 0.159.0"; exit 1`, Version{Unknown: VersionNotRead}},
		{`echo nothing`, Version{Unknown: VersionNotRead}},
		{`sleep 30`, Version{Unknown: VersionNotRead}},
		{`trap '' TERM; while :; do sleep 1; done`, Version{Unknown: VersionNotRead}},
	} {
		got, err := ReadVersion(script(t, tc.body), nil, "")
		if err != nil || got != tc.want {
			t.Fatalf("%q: got %+v %v, want %+v", tc.body, got, err, tc.want)
		}
	}
	got, err := ReadVersion(filepath.Join(t.TempDir(), "absent"), nil, "")
	if err != nil || got != (Version{Unknown: VersionNotRead}) {
		t.Fatalf("a program that cannot start: got %+v %v", got, err)
	}
}

// A read whose holder cannot show its tree ended refuses the launch: the
// unknown version is never taken from a check that may have left a process.
func TestAVersionReadThatCannotEndRefuses(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	program := script(t, `echo $$ > '`+pidFile+`'; kill -9 $PPID; echo "codex-cli 0.159.0"; sleep 30`)
	t.Cleanup(func() {
		if raw, err := os.ReadFile(pidFile); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	})
	got, err := ReadVersion(program, nil, "")
	var failed *CheckFailedError
	if !errors.As(err, &failed) || failed.Label != "--version" || failed.Outcome != OutcomeNotEnded || got != (Version{}) {
		t.Fatalf("got %+v %v, want the check's failure", got, err)
	}
}

func TestAnUnknownVersionNamesItsCause(t *testing.T) {
	for _, tc := range []struct {
		version Version
		note    string
	}{
		{Version{Value: "0.159.0"}, ""},
		{Version{}, "harness version unknown (not read)"},
		{Version{Unknown: VersionNotOnPath}, "harness version unknown (no claude on PATH)"},
		{Version{Unknown: VersionNotInPath}, "harness version unknown (path names no version)"},
	} {
		if got := tc.version.Note(); got != tc.note {
			t.Fatalf("%+v: %q, want %q", tc.version, got, tc.note)
		}
	}
}

// Claude Code's version is the path its PATH's claude resolves to, read
// without running it.
func TestClaudeCodesVersionIsItsPath(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "ran")
	install := func(dir, target string) string {
		t.Helper()
		bin := filepath.Join(root, dir)
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(bin, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(bin, "claude")); err != nil {
			t.Fatal(err)
		}
		return bin
	}
	native := install("native", filepath.Join(root, "share", "claude", "versions", "2.1.284"))
	npm := install("npm", filepath.Join(root, "lib", "node_modules", "cli.js"))
	empty := filepath.Join(root, "empty")
	if err := os.MkdirAll(filepath.Join(empty, "claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path string
		want Version
	}{
		{native, Version{Value: "2.1.284"}},
		{empty + ":" + native, Version{Value: "2.1.284"}},
		{npm + ":" + native, Version{Unknown: VersionNotInPath}},
		{empty, Version{Unknown: VersionNotOnPath}},
		{"", Version{Unknown: VersionNotOnPath}},
	} {
		if got := ClaudeVersion([]string{"PATH=" + tc.path}); got != tc.want {
			t.Fatalf("PATH=%s: %+v, want %+v", tc.path, got, tc.want)
		}
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the version was read by running claude")
	}
}

// A gate or a bound recorded for a version holds for that version alone.
func TestGatesAndBoundsHoldPerVersion(t *testing.T) {
	for _, tc := range []struct {
		harness, version string
		g1, g7           bool
		bound            int64
	}{
		{"codex", "0.159.0", true, false, 861},
		{"codex", "0.159.1", false, false, 0},
		{"codex", "", false, false, 0},
		{"claude", "2.1.284", false, true, 2048},
		{"claude", "2.1.285", false, false, 0},
		{"claude", "0.159.0", false, false, 0},
	} {
		gates := ResolveGates(tc.harness, tc.version, nil)
		bound, ok := gates.OutputBound()
		if !gates.Open(GateG1) != tc.g1 || !gates.Open(GateG7) != tc.g7 || bound != tc.bound || ok != (tc.bound > 0) {
			t.Fatalf("%s %q: G1 closed %v, G7 closed %v, bound %d %v", tc.harness, tc.version, !gates.Open(GateG1), !gates.Open(GateG7), bound, ok)
		}
	}
	// An assumed gate is closed for any version, and carries no bound.
	gates := ResolveGates("codex", "", []string{GateL5, GateG2})
	if _, ok := gates.OutputBound(); ok || gates.Open(GateG2) {
		t.Fatal("an assumption gave a bound, or left its gate open")
	}
	if !GatesNeedVersion("codex") || !GatesNeedVersion("claude") || GatesNeedVersion("other") {
		t.Fatal("which harnesses need a version is not the table's")
	}
}
