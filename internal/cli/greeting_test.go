package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

func TestGreetingAndIntroHaveIndependentSwitches(t *testing.T) {
	for _, args := range [][]string{{"--no-greeting", "codex"}, {"--no-intro", "claude"}} {
		if _, err := parse(args); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBootstrapReadyDoesNotReportOnEarlyWork(t *testing.T) {
	dir := liveSession(t, "api")
	peer := otherRun(t, dir, "web")
	readFrom(t, dir, peer)
	marker := filepath.Join(state.InboxPath(dir, "api"), "greeting")
	if err := os.WriteFile(marker, []byte(epochOf(t, dir, "api")), 0o600); err != nil {
		t.Fatal(err)
	}
	run("turn-ended", `{"type":"agent-turn-complete","last-assistant-message":"ready"}`)
	if len(finishedFor(t, dir, "web")) != 0 || len(inbox.Waiters(dir, "api", epochOf(t, dir, "api"))) != 1 {
		t.Fatal("ready settled a task that arrived during bootstrap")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("bootstrap marker left: %v", err)
	}
	run("turn-ended", turnPayload)
	if len(finishedFor(t, dir, "web")) != 1 {
		t.Fatal("real work result was suppressed")
	}
}

func TestGreetingRetryDoesNotFinishEarlyWork(t *testing.T) {
	dir := liveSession(t, "api")
	peer := otherRun(t, dir, "web")
	readFrom(t, dir, peer)
	if err := inbox.MarkGreeting(dir, "api", epochOf(t, dir, "api")); err != nil {
		t.Fatal(err)
	}
	payload := `{"type":"agent-turn-complete","turn-id":"bootstrap","last-assistant-message":"ready"}`
	run("turn-ended", payload)
	run("turn-ended", payload)
	files := finishedFor(t, dir, "web")
	waits := inbox.Waiters(dir, "api", epochOf(t, dir, "api"))
	t.Logf("repeated bootstrap: reports=%d remaining waits=%d", len(files), len(waits))
	if len(files) != 0 || len(waits) != 1 {
		t.Fatal("duplicate ready callback completed real work")
	}
}

func TestBootstrapReceiptFailureKeepsTheGreetingPending(t *testing.T) {
	dir := liveSession(t, "api")
	peer := otherRun(t, dir, "web")
	readFrom(t, dir, peer)
	epoch := epochOf(t, dir, "api")
	if err := inbox.MarkGreeting(dir, "api", epoch); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(state.InboxPath(dir, "api"), "turns")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	payload := `{"turn-id":"bootstrap","last_assistant_message":"ready"}`
	run("turn-ended", payload)
	if pending, err := inbox.GreetingPending(dir, "api", epoch); err != nil || !pending {
		t.Fatalf("uncommitted greeting was consumed: pending=%v err=%v", pending, err)
	}
	if err := os.Remove(blocked); err != nil {
		t.Fatal(err)
	}
	run("turn-ended", payload)
	run("turn-ended", payload)
	if len(finishedFor(t, dir, "web")) != 0 || len(inbox.Waiters(dir, "api", epoch)) != 1 {
		t.Fatal("bootstrap retry settled early work")
	}
	run("turn-ended", `{"turn-id":"work","last_assistant_message":"work result"}`)
	if len(finishedFor(t, dir, "web")) != 1 || len(inbox.Waiters(dir, "api", epoch)) != 0 {
		t.Fatal("next real result was lost")
	}
}
