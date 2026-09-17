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
