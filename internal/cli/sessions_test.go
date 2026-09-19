package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/proc"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// liveSession publishes a session served by nobody, in an isolated state
// directory. This process stands in for the wrapper: it is alive, so the record
// is alive, which is all the sender needs to accept a message.
func liveSession(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Setenv(state.DirEnv, dir)
	// The tests may run inside a rewake session themselves; its name and run
	// must not leak into the sessions they make up.
	t.Setenv(state.SessionEnv, "")
	t.Setenv(state.EpochEnv, "")
	resolved, err := state.Dir()
	if err != nil {
		t.Fatalf("state.Dir: %v", err)
	}

	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatalf("start time: %v", err)
	}
	session := registry.Session{
		Name:         name,
		Harness:      "claude",
		ServicePID:   os.Getpid(),
		ServiceStart: start,
		CWD:          resolved,
		StartedAt:    time.Now(),
		Socket:       filepath.Join(resolved, "sock", name+".sock"),
	}
	if err := registry.Publish(resolved, session); err != nil {
		t.Fatalf("publish: %v", err)
	}
	// A process of the session carries its run, as one started by the wrapper does.
	t.Setenv(state.EpochEnv, session.Epoch())
	return resolved
}

func TestListShowsALiveSession(t *testing.T) {
	liveSession(t, "api")

	code, out, errOut := run("list")
	if code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, errOut)
	}
	// The empty answer names a session too, in its hint, so the test asks for
	// the line that only a listed session produces.
	if strings.Contains(out, "No sessions are running") {
		t.Fatalf("list reported nothing running: %q", out)
	}
	if !strings.Contains(out, "Room: default") || !strings.Contains(out, "Harness: \"claude\"") || !strings.Contains(out, "api") {
		t.Errorf("first line = %q, want the session name and its harness", out)
	}
}

func TestSendToUnknownSessionNamesTheLiveOnes(t *testing.T) {
	liveSession(t, "api")

	code, _, errOut := run("send", "web", "hello")
	if code != ExitUsage {
		t.Fatalf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errOut, "No session named \"web\"") {
		t.Errorf("refusal does not name the target: %s", errOut)
	}
	if !strings.Contains(errOut, "Running now: api") {
		t.Errorf("refusal does not say who is reachable: %s", errOut)
	}
}

// Nothing serves the mailbox here, so the message is accepted and stays
// pending: exit code 3, and the message waiting on disk for whoever serves it.
func TestSendWithoutAServerIsPending(t *testing.T) {
	dir := liveSession(t, "api")

	code, out, errOut := run("send", "api", "hello", "--wait", "0.3")
	if code != ExitPending {
		t.Fatalf("exit = %d, want %d (stderr: %s)", code, ExitPending, errOut)
	}
	if !strings.Contains(out, "pending for api") {
		t.Errorf("stdout does not explain the pending result: %q", out)
	}

	waiting, err := filepath.Glob(filepath.Join(state.InboxPath(dir, "api"), "*.json"))
	if err != nil || len(waiting) != 1 {
		t.Fatalf("mailbox holds %v, want exactly one message (%v)", waiting, err)
	}
	raw, err := os.ReadFile(waiting[0])
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var written struct {
		To      string `json:"to"`
		From    string `json:"from"`
		Text    string `json:"text"`
		ToEpoch string `json:"toEpoch"`
	}
	if err := json.Unmarshal(raw, &written); err != nil {
		t.Fatalf("the waiting message is not readable: %v", err)
	}
	if written.Text != "hello" || written.To != "api" {
		t.Errorf("message = %+v, want the text and target as sent", written)
	}
	if written.ToEpoch == "" {
		t.Error("the message carries no epoch, so a later session with this name would receive it")
	}
}

func TestSendRefusesEmptyText(t *testing.T) {
	liveSession(t, "api")

	code, _, errOut := run("send", "api", "   ")
	if code != ExitUsage {
		t.Fatalf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errOut, "The message is empty.") {
		t.Errorf("unexpected refusal: %s", errOut)
	}
}

func TestWhoamiOutsideASession(t *testing.T) {
	liveSession(t, "api")
	t.Setenv(state.SessionEnv, "")

	code, out, _ := run("whoami")
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out, "not part of a rewake session") {
		t.Errorf("whoami does not say the shell is unnamed: %q", out)
	}
}
