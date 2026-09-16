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

	"github.com/iiiokojiadbi/rewake/internal/proc"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// epochEnv is the variable a wrapper gives its harness to say which run of the
// name it is. Spelled out here so these tests read the same on older code.
const epochEnv = "REWAKE_EPOCH"

// otherRun publishes a live session under a name, backed by a process of its
// own, so it is a different run from the test process.
func otherRun(t *testing.T, dir, name string) registry.Session {
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
		Name: name, Harness: "claude", ServicePID: child.Process.Pid, ServiceStart: start,
		CWD: dir, StartedAt: time.Now(),
	}
	_ = os.Remove(state.SessionPath(dir, name))
	if err := registry.Publish(dir, session); err != nil {
		t.Fatalf("publish: %v", err)
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

// Marking a message read before its text reached the reader loses it.
func TestInboxKeepsMailItCouldNotShow(t *testing.T) {
	dir := liveSession(t, "api")
	t.Setenv(state.SessionEnv, "api")
	current, _ := registry.Lookup(dir, "api")
	t.Setenv(epochEnv, current.Epoch())
	id := rawUnread(t, dir, "api", map[string]any{"from": "web", "toEpoch": current.Epoch(), "text": "keep me"})

	code := Run([]string{"inbox"}, brokenWriter{}, brokenWriter{})
	if code == ExitOK {
		t.Error("inbox reported success with nowhere to write")
	}
	if _, err := os.Stat(filepath.Join(state.UnreadPath(dir, "api"), id+".json")); err != nil {
		t.Errorf("a message nobody saw left the unread set: %v", err)
	}
}

// readFrom has api read a message from the given run of web.
func readFrom(t *testing.T, dir string, web registry.Session) {
	t.Helper()
	current, _ := registry.Lookup(dir, "api")
	t.Setenv(state.SessionEnv, "api")
	t.Setenv(epochEnv, current.Epoch())
	rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": current.Epoch(), "text": "rerun"})
	if code, _, errOut := run("inbox"); code != ExitOK {
		t.Fatalf("inbox: %d %s", code, errOut)
	}
}

func finishedFor(t *testing.T, dir, name string) []string {
	t.Helper()
	found, _ := filepath.Glob(filepath.Join(state.InboxPath(dir, name), "*.json"))
	return found
}

// The report goes to the run that wrote, not to whoever holds the name when the
// turn ends.
func TestTurnEndSkipsANameThatChangedHands(t *testing.T) {
	dir := liveSession(t, "api")
	first := otherRun(t, dir, "web")
	readFrom(t, dir, first)
	otherRun(t, dir, "web")

	run("turn-ended", turnPayload)
	if found := finishedFor(t, dir, "web"); len(found) != 0 {
		t.Errorf("web holds %v; the report went to a run that never wrote", found)
	}
}

// What an earlier run of this name read is not this run's to report.
func TestTurnEndIgnoresAnEarlierRunsReading(t *testing.T) {
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	readFrom(t, dir, web)

	t.Setenv(epochEnv, "7.7")
	run("turn-ended", turnPayload)
	if found := finishedFor(t, dir, "web"); len(found) != 0 {
		t.Errorf("web holds %v; a later run reported a turn it did not have", found)
	}
}

// A report that could not be written must still be owed.
func TestTurnEndKeepsTheWaitWhenTheReportFails(t *testing.T) {
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	readFrom(t, dir, web)

	mailbox := state.InboxPath(dir, "web")
	_ = os.RemoveAll(mailbox)
	if err := os.WriteFile(mailbox, nil, 0o600); err != nil {
		t.Fatalf("block: %v", err)
	}
	run("turn-ended", turnPayload)
	if err := os.Remove(mailbox); err != nil {
		t.Fatalf("unblock: %v", err)
	}

	run("turn-ended", turnPayload)
	if found := finishedFor(t, dir, "web"); len(found) != 1 {
		t.Errorf("web holds %v after the mailbox came back, want the owed report", found)
	}
}

// A hook that gets no payload must not wait for one forever.
func TestTurnEndDoesNotWaitOnAnOpenPipe(t *testing.T) {
	liveSession(t, "api")
	t.Setenv(state.SessionEnv, "api")
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer writer.Close()
	previous := os.Stdin
	os.Stdin = reader
	defer func() { os.Stdin = previous }()

	done := make(chan struct{})
	go func() {
		run("turn-ended")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("turn-ended is still waiting on a pipe that never closes")
	}
}

// A direct answer that could not be written leaves the report owed.
func TestAFailedAnswerKeepsTheWait(t *testing.T) {
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	readFrom(t, dir, web)

	mailbox := state.InboxPath(dir, "web")
	_ = os.RemoveAll(mailbox)
	if err := os.WriteFile(mailbox, nil, 0o600); err != nil {
		t.Fatalf("block: %v", err)
	}
	if code, _, _ := run("send", "web", "green", "--wait", "0"); code == ExitOK {
		t.Fatal("send succeeded into a mailbox that cannot exist")
	}
	current, _ := registry.Lookup(dir, "api")
	if waiting := inboxWaiters(dir, current.Epoch()); len(waiting) != 1 {
		t.Errorf("waiting = %v after a failed answer, want web still owed", waiting)
	}
}
