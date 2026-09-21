package workflow

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// These come from the reproductions written during the stage-0 acceptance
// review. They are kept because each one is a way a case could look green
// while the run was not, which is the single thing this package must never do.

func TestExpiredCaseCannotPass(t *testing.T) {
	rec := &recorder{}
	c := newCase(rec, Spec{Name: "expired", Observations: []string{"a"}, Deadline: time.Millisecond})
	<-c.Context().Done()
	c.Observed("a", "recorded after the deadline")
	rec.finish()
	if outcome, reason := c.Result(); outcome.Green() {
		t.Errorf("a case that ran out of time is green: outcome=%s reason=%q", outcome, reason)
	}
}

func TestRemovalFailureCannotKeepPass(t *testing.T) {
	rec := &recorder{}
	c := newCase(rec, Spec{Name: "removal", Observations: []string{"a"}})
	c.Observed("a", "done")
	c.RemoveOnFinish(filepath.Join(t.TempDir(), "invalid\x00path"))
	rec.finish()
	outcome, reason := c.Result()
	if outcome.Green() {
		t.Errorf("a case that could not clean up is green: outcome=%s reason=%q errors=%v", outcome, reason, rec.errors)
	}
	if len(rec.errors) == 0 {
		t.Error("the removal failure was never reported")
	}
}

func TestFailureReportedBeforeClassificationCannotKeepPass(t *testing.T) {
	rec := &recorder{}
	c := newCase(rec, Spec{Name: "late-error", Observations: []string{"a"}})
	c.Observed("a", "done")
	rec.Errorf("late endpoint error")
	rec.finish()
	if outcome, reason := c.Result(); outcome.Green() {
		t.Errorf("a case whose test already failed is green: outcome=%s reason=%q", outcome, reason)
	}
}

// The session-record check accepts the registry's own lock file and nothing
// else. Narrowing it to "only .json" would look equivalent and would quietly
// stop catching the half-written record an interrupted session leaves behind.
func TestUnfinishedSessionRecordIsNotAcceptedAsClean(t *testing.T) {
	iso := &Isolation{StateDir: t.TempDir()}
	dir := filepath.Join(iso.StateDir, "rooms", "default", "sessions")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	c := newCase(&recorder{}, Spec{Name: "records", Observations: []string{"a"}})
	iso.registerCleanupChecks(c)
	recordCheck := func() error {
		for _, check := range c.checks {
			if check.what == "no session record left behind" {
				return check.check()
			}
		}
		t.Fatal("the session-record check is gone")
		return nil
	}

	// The lock outlives every session by design.
	if err := os.WriteFile(filepath.Join(dir, ".worker-codex.lock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := recordCheck(); err != nil {
		t.Errorf("the registry's own lock file counted as a leftover: %v", err)
	}

	// A temporary file from an interrupted atomic write does not.
	if err := os.WriteFile(filepath.Join(dir, ".tmp-worker-codex-1234"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := recordCheck(); err == nil {
		t.Error("a half-written session record went unnoticed")
	}
}

// The server keeps these beside its socket in the ordinary course of events,
// and removes only the sockets themselves. A cleanup check that called them
// leftovers would fail the first working Codex scenario over nothing.
func TestOrdinaryServerArtifactsAreNotLiveSockets(t *testing.T) {
	iso := &Isolation{StateDir: t.TempDir()}
	dir := filepath.Join(iso.StateDir, "rooms", "default", "sock")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("worker.sock.up.log", "")
	write("worker.sock.gateway.log", "")
	write("worker.sock.outcomes.json", "[]")

	if err := iso.noLiveSockets(); err != nil {
		t.Errorf("ordinary retained artifacts counted as live sockets: %v", err)
	}
	if err := iso.noPendingOutcomes(); err != nil {
		t.Errorf("an empty completion journal counted as unpublished work: %v", err)
	}

	// A journal with entries is the opposite case: results the session never
	// published, which is a case that did not finish.
	write("worker.sock.outcomes.json", `[{"ID":"1"}]`)
	if err := iso.noPendingOutcomes(); err == nil {
		t.Error("outcomes left unpublished in the journal went unnoticed")
	}

	// A surviving socket is still a leftover — and it has to be a real one,
	// or the check would be passing on a name again.
	listener, err := net.Listen("unix", filepath.Join(dir, "worker.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	if err := iso.noLiveSockets(); err == nil {
		t.Error("a surviving socket went unnoticed")
	}
}
