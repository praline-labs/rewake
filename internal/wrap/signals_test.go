package wrap

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The wrapper drops Ctrl+C for itself. The harness must not inherit that: an
// ignored signal stays ignored across exec, and the agent and every command it
// ran would have lost Ctrl+C with it.
func TestKeyboardSignalsAreNotIgnoredInTheHarness(t *testing.T) {
	dir := stateDir(t)
	out := filepath.Join(t.TempDir(), "status")
	fake := &fakeHarness{script: "grep '^SigIgn:' /proc/self/status > " + out}

	if _, err := Run(context.Background(), Request{Harness: fake, Dir: dir, Name: "api"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	mask, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(string(raw), "SigIgn:")), 16, 64)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	const sigint, sigquit = 1 << (2 - 1), 1 << (3 - 1)
	if mask&(sigint|sigquit) != 0 {
		t.Errorf("the harness starts with SigIgn %#x: Ctrl+C or Ctrl+\\ would do nothing in it", mask)
	}
}

// Ctrl+Z stops the whole job and "fg" continues it; the report of the harness's
// stop is read only afterwards. Stopping again then would hand the shell a
// stopped job while the harness runs on its own.
func TestAStopAlreadyOverIsNotFollowed(t *testing.T) {
	stops := 0
	followStop(os.Getpid(), func(int) bool { return false }, func() { stops++ })
	if stops != 0 {
		t.Errorf("the wrapper stopped for a harness that is running again")
	}
}

func TestAStopInProgressIsFollowed(t *testing.T) {
	stops := 0
	// Our own pid: continuing a running process is harmless.
	followStop(os.Getpid(), func(int) bool { return true }, func() { stops++ })
	if stops != 1 {
		t.Errorf("the wrapper did not stop with a harness that is stopped")
	}
}
