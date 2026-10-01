package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/receipt"
	"github.com/praline-labs/rewake/internal/state"
)

// Rule 6 (docs/mail-bridge-cli.md#the-rules-the-code-holds): a check that
// could not be made stops the effect it decides and leaves it to be taken up
// again. The unknown heads-up is in bridge_rules_test.go, the mailbox that
// cannot be searched in the inbox package.

// A read refused for a letter that is not there answers the same refusal to
// the same words in its turn.
func TestARefusedReadIsReplayedInItsTurn(t *testing.T) {
	toolSession(t)
	tool := newToolCaller(t)
	first := tool.run("inbox", "--message", "missing-id")
	if first.code != ExitFailed {
		t.Fatalf("first refusal: %+v", first)
	}
	second := tool.run("inbox", "--message", "missing-id")
	if second.code != first.code || second.errOut != first.errOut || !strings.Contains(second.out, "ran earlier") {
		t.Fatalf("not replayed: first %+v, second %+v", first, second)
	}
}

// A letter that could not be looked up before its first part is not shown
// from what was frozen, which may be a text withdrawn since; once the mailbox
// reads again, the same words show what it holds.
func TestALetterThatCannotBeLookedUpIsNotShown(t *testing.T) {
	dir, self, web := toolSession(t)
	rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": self.Epoch(), "kind": "task", "text": longText})
	time.Sleep(2 * time.Millisecond)
	secret := strings.Repeat("WITHDRAWN_SECRET ", 500)
	rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": self.Epoch(), "kind": "task", "text": secret})
	tool := newToolCaller(t)
	first := tool.run("inbox")
	words := nextFrom(first.out)
	if words == nil {
		t.Fatal(first)
	}
	token, _, _ := strings.Cut(words[2], ".")
	if err := withdrawLocked(dir, letterWith(t, dir, self, secret)); err != nil {
		t.Fatal(err)
	}
	unread := state.UnreadPath(dir, "api")
	if err := os.Chmod(unread, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unread, 0o700) })
	blind := tool.run("inbox", "--next", token+".1.0")
	if blind.code == ExitOK || strings.Contains(blind.out, "WITHDRAWN_SECRET") {
		t.Fatalf("a letter that could not be looked up was shown: %+v", blind.code)
	}
	if err := os.Chmod(unread, 0o700); err != nil {
		t.Fatal(err)
	}
	again := tool.run("inbox", "--next", token+".1.0")
	if again.code != ExitOK || strings.Contains(again.out, "WITHDRAWN_SECRET") || !strings.Contains(again.out, "withdrew") {
		t.Fatalf("the same words once the mailbox reads: %+v", again)
	}
}

// A waiter that cannot be read may be owed: "nothing owed" is no answer then,
// and the mark stays open for rewake retry instead of being refused for good.
func TestAWaiterThatCannotBeReadLeavesTheMarkOpen(t *testing.T) {
	dir, self, web := toolSession(t)
	readFrom(t, dir, web)
	turnStarted(t, dir, self, markAt-1)
	waiter := filepath.Join(state.AwaitingPath(dir, "api"), self.Epoch(), "web")
	if err := os.Chmod(waiter, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(waiter, 0o600) })
	tool := newToolCaller(t)
	blind := tool.run("pending", "the suite is running")
	if blind.code != ExitFailed || !strings.Contains(blind.errOut, "rewake retry ") {
		t.Fatalf("a mark past a waiter it could not read: %+v", blind)
	}
	token := blind.errOut[strings.Index(blind.errOut, "rewake retry ")+len("rewake retry "):][:24]
	if err := os.Chmod(waiter, 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out, errOut := run("retry", token); code != ExitOK || !strings.Contains(out, "marked pending") {
		t.Fatalf("retry once the waiter reads: %d %s %s", code, out, errOut)
	}
}

// A recipient whose record cannot be read may still be the run the heads-up
// was pinned to: that is no ended run, and the operation stays open.
func TestARecipientThatCannotBeLookedUpIsNotAnEndedRun(t *testing.T) {
	dir, self, _ := toolSession(t)
	words := []string{"send", "web", "look when you can", "--notify", "--wait", "0"}
	call, ctx, _, _ := expiredCall(t, words...)
	_ = handleSend(ctx, call)
	records := unresolved(t, dir, self.Epoch(), ctx.scope.digest)
	if len(records) != 1 {
		t.Fatal(records)
	}
	session := state.SessionPath(dir, "web")
	if err := os.Chmod(session, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(session, 0o600) })
	tool := newToolCaller(t)
	if blind := tool.run(words...); blind.code != ExitFailed || !strings.Contains(blind.errOut, "rewake retry "+records[0].Token) {
		t.Fatalf("a heads-up past a recipient it could not look up: %+v", blind)
	}
	record, err := receipt.Load(dir, "api", self.Epoch(), records[0].Token)
	if err != nil || record.Phase != receipt.Open || record.Uncertain {
		t.Fatalf("record %+v, %v", record, err)
	}
	if err := os.Chmod(session, 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out, errOut := run("retry", records[0].Token); code == ExitFailed || code == ExitUsage {
		t.Fatalf("retry once the recipient reads: %d %s %s", code, out, errOut)
	}
	if letters := headsUps(t, dir, "look when you can"); len(letters) != 1 {
		t.Fatalf("published %d letters", len(letters))
	}
}

// must is a lookup's answer in a test that set up nothing it could not read.
func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}
