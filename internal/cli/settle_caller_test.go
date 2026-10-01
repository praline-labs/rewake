package cli

import (
	"os"
	"testing"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/state"
)

// asPerson runs a command from a shell outside every session, as the person
// settling a stop does.
func asPerson(t *testing.T, words ...string) (int, string, string) {
	t.Helper()
	session := os.Getenv(state.SessionEnv)
	t.Setenv(state.SessionEnv, "")
	defer func() { _ = os.Setenv(state.SessionEnv, session) }()
	return run(words...)
}

// The word on a stopped mailbox is main's or the person's: a worker that
// settled its own stop would decide the very evidence it is stopped on
// (docs/turn-end-recovery.md#the-stop-and-rewake-settle).
func TestOnlyMainOrThePersonSettles(t *testing.T) {
	dir, self, web := toolSession(t)
	lead := otherRun(t, dir, "lead")
	markMain(t, dir, "lead")
	task := earlierTask(t, dir, web)
	report := writeEarlierBuildReceipt(t, dir, web, task, false)
	if completeTurn(dir, self, turnResult{Text: "answer", Ended: 10}, "") == nil {
		t.Fatal("an unknown report did not stop the mailbox")
	}
	if code, out, errOut := run("settle", "api", report.ID, "--delivered"); code != ExitUsage {
		t.Fatalf("a worker settled its own stop: %d %s %s", code, out, errOut)
	}
	if inbox.MailboxStopped(dir, "api") == nil {
		t.Fatal("a refused settle let the stop go")
	}
	t.Setenv(state.SessionEnv, "lead")
	t.Setenv(epochEnv, lead.Epoch())
	if code, out, errOut := run("settle", "api", report.ID, "--delivered"); code != ExitOK {
		t.Fatalf("main could not settle: %d %s %s", code, out, errOut)
	}
	if inbox.MailboxStopped(dir, "api") != nil {
		t.Fatal("main's settle did not lift the stop")
	}
}
