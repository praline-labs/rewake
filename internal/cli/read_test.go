package cli

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/registry/registrytest"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// epochEnv is the variable a wrapper gives its harness to say which run of the
// name it is. Spelled out here so these tests read the same on older code.
const epochEnv = "REWAKE_EPOCH"

// otherRun publishes a live session under a name, backed by a process of its
// own, so it is a different run from the test process.
func otherRun(t *testing.T, dir, name string) registry.Session {
	t.Helper()
	session := otherSession(t, dir, name)
	_ = os.Remove(state.SessionPath(dir, name))
	if err := registry.Publish(dir, session); err != nil {
		t.Fatalf("publish: %v", err)
	}
	return session
}

// otherSession describes a live run of name in dir, a process of its own,
// without publishing it.
func otherSession(t *testing.T, dir, name string) registry.Session {
	t.Helper()
	child := exec.Command("sleep", "60")
	if err := child.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = child.Process.Kill(); _, _ = child.Process.Wait() })
	start, err := proc.StartTime(child.Process.Pid)
	if err != nil {
		t.Fatalf("start time: %v", err)
	}
	session := registry.Session{
		Name: name, Harness: agentID, ServicePID: child.Process.Pid, ServiceStart: start, Boot: registrytest.Boot(t),
		PIDNamespace: proc.Namespace(), CWD: dir, StartedAt: time.Now(), Live: allLive(),
	}
	return session
}

// rawUnread leaves a message, written as raw JSON, where an announced one waits.
func rawUnread(t *testing.T, dir, to string, fields map[string]any) string {
	t.Helper()
	id := time.Now().Format("20060102150405.000000000")
	fields["id"], fields["to"], fields["createdAt"] = id, to, time.Now()
	for _, path := range []string{state.InboxPath(dir, to), state.UnreadPath(dir, to)} {
		if err := state.EnsureSubdir(path); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	encoded, _ := json.Marshal(fields)
	if err := os.WriteFile(filepath.Join(state.UnreadPath(dir, to), id+".json"), encoded, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return id
}

// A read message has been delivered and then some. Calling that a failure sends
// the sender to do the whole thing again.
func TestSendCountsAReadMessageAsDelivered(t *testing.T) {
	dir := liveSession(t, "api")
	go func() {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			found, _ := filepath.Glob(filepath.Join(state.InboxPath(dir, "api"), "*.json"))
			if len(found) == 1 {
				status := strings.TrimSuffix(found[0], ".json") + ".status"
				_ = os.WriteFile(status, []byte(`{"state":"read","at":"2026-09-16T00:00:00Z"}`), 0o600)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	code, out, errOut := run("send", "api", "hello", "--wait", "2")
	if code != ExitOK {
		t.Errorf("exit = %d (%q %q), want success for a message already read", code, out, errOut)
	}
}

// A shell left behind by a session that ended must not read the mail of the
// session that took the name next.
func TestInboxRefusesAnEarlierRun(t *testing.T) {
	dir := liveSession(t, "api")
	t.Setenv(state.SessionEnv, "api")
	t.Setenv(epochEnv, "1.1")
	current, _ := registry.Lookup(dir, "api")
	id := rawUnread(t, dir, "api", map[string]any{"from": "web", "toEpoch": current.Epoch(), "text": "for the new run"})

	code, out, _ := run("inbox")
	if code == ExitOK || strings.Contains(out, "for the new run") {
		t.Errorf("exit = %d, out = %q; an earlier run read the current run's mail", code, out)
	}
	if _, err := os.Stat(filepath.Join(state.UnreadPath(dir, "api"), id+".json")); err != nil {
		t.Errorf("the mail was taken: %v", err)
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("no space left on device") }

// A process with a session name and no run cannot be told from a leftover of
// an ended session, so it reads nothing.
func TestInboxWithoutARunIsRefused(t *testing.T) {
	dir := liveSession(t, "api")
	t.Setenv(state.SessionEnv, "api")
	t.Setenv(epochEnv, "")
	rawUnread(t, dir, "api", map[string]any{"from": "web", "toEpoch": epochOf(t, dir, "api"), "text": "for the current run"})

	code, out, _ := run("inbox")
	if code == ExitOK || strings.Contains(out, "for the current run") {
		t.Errorf("exit = %d, out = %q; a process without a run read the mail", code, out)
	}
}

// block closes a directory to writes, until unblock opens it.
func block(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("block: %v", err)
	}
	if err := os.Chmod(path, 0o500); err != nil {
		t.Fatalf("block: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o700) })
}

func unblock(t *testing.T, path string) {
	t.Helper()
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatalf("unblock: %v", err)
	}
}
