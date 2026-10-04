package codex

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/proc"
)

// A --command wrapper may end before a child of the group it leads, and the
// child may ignore SIGTERM: the withdrawal ends the whole group before the
// server is started again
// (docs/mail-bridge-launch-codex.md#the-version-confirmed-at-start).
func TestRetiringEndsTheWholeFirstGroup(t *testing.T) {
	retireGrace = 300 * time.Millisecond
	t.Cleanup(func() { retireGrace = 2 * time.Second })
	dir := t.TempDir()
	childFile := filepath.Join(dir, "child")
	server := newServer(filepath.Join(dir, "s.sock"), nil, os.Environ(), dir)
	server.program = "/bin/sh"
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The wrapper starts a child that ignores SIGTERM, then ends at SIGTERM.
	retired, err := server.spawn([]string{"-c", `sh -c 'trap "" TERM; echo $$ > "$1"; exec sleep 20' child "$1" & wait`, "wrapper", childFile}, cancel)
	if err != nil {
		t.Fatal(err)
	}
	child := 0
	for deadline := time.Now().Add(2 * time.Second); child == 0 && time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		raw, _ := os.ReadFile(childFile)
		child, _ = strconv.Atoi(strings.TrimSpace(string(raw)))
	}
	if child == 0 {
		t.Fatal("the child did not start")
	}
	t.Cleanup(func() { _ = syscall.Kill(child, syscall.SIGKILL) })
	if err := server.retire(retired); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(child), "stat")); err == nil {
		if _, after, _ := strings.Cut(string(raw), ") "); !strings.HasPrefix(after, "Z ") {
			t.Fatalf("retire returned while the group's child %d still ran: %s", child, after)
		}
	}
}

// A group that does not end within the bound refuses the start without the
// tool: the second server would run beside the first, and the socket it
// would take is left where it is.
func TestAGroupThatDoesNotEndRefusesTheSecondStart(t *testing.T) {
	retireGrace = 200 * time.Millisecond
	groupAlive = func(int) bool { return true }
	t.Cleanup(func() { retireGrace, groupAlive = 2*time.Second, proc.GroupAlive })
	var withdrawn confirmCase
	for _, tc := range confirmCases() {
		if tc.withdrawn {
			withdrawn = tc
			break
		}
	}
	server, starts, err := startConfirmed(t, withdrawn)
	if err == nil || !strings.Contains(err.Error(), "did not end") {
		t.Fatalf("the start went on past a group that did not end: %v", err)
	}
	if len(starts) != 1 || server.ToolWithdrawn() != "" {
		t.Fatalf("starts %q, withdrawn %q: a second server was started", starts, server.ToolWithdrawn())
	}
	if _, err := os.Stat(server.upstream); err != nil {
		t.Fatalf("the first server's socket was removed: %v", err)
	}
}
