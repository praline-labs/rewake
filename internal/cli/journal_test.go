package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/receipt"
	"github.com/praline-labs/rewake/internal/state"
)

// headsUps are the letters with this text waiting in web's mailbox.
func headsUps(t *testing.T, dir, text string) []inbox.Message {
	t.Helper()
	var found []inbox.Message
	files, _ := filepath.Glob(filepath.Join(state.InboxPath(dir, "web"), "*.json"))
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var message inbox.Message
		if json.Unmarshal(raw, &message) == nil && message.Text == text {
			found = append(found, message)
		}
	}
	return found
}

// A heads-up repeated in one turn — the harness retried, or the model called
// again — is published once, and the repeat gets the first call's answer.
func TestAHeadsUpRepeatedInOneTurnIsPublishedOnce(t *testing.T) {
	dir, _, _ := toolSession(t)
	tool := newToolCaller(t)
	first := tool.run("send", "web", "look", "--notify", "--wait", "0")
	if first.code == ExitUsage || first.code == ExitFailed {
		t.Fatalf("first: %+v", first)
	}
	again := tool.run("send", "--notify", "--wait=0", "web", "look")
	if again.code != first.code || !strings.Contains(again.out, "ran earlier") {
		t.Fatalf("repeat: %+v", again)
	}
	if letters := headsUps(t, dir, "look"); len(letters) != 1 {
		t.Fatalf("one turn published %d letters", len(letters))
	}
	tool.turn = "turn-2"
	if next := tool.run("send", "web", "look", "--notify", "--wait", "0"); strings.Contains(next.out, "ran earlier") {
		t.Fatalf("another turn joined: %+v", next)
	}
	if letters := headsUps(t, dir, "look"); len(letters) != 2 {
		t.Fatalf("two turns published %d letters", len(letters))
	}
}

// expiredCall is a tool call whose answer stopped being wanted before its
// heads-up was published: what a harness timeout leaves behind.
func expiredCall(t *testing.T, words ...string) (Call, *Context, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	result, err := toolCall(words)
	if err != nil {
		t.Fatal(err)
	}
	normal := normalized(result)
	now := boottime.Now()
	var out, errOut bytes.Buffer
	ctx := &Context{Stdout: &out, Stderr: &errOut, scope: &callScope{
		ticket: bridge.Ticket{Conversation: "conversation-1", Turn: "turn-1", CallID: "late", CalledBoot: now - 2, DeadlineBoot: now - 1, Transport: "test-tool", WordsDigest: bridge.Digest(normal)},
		words:  normal, digest: bridge.Digest(normal),
	}}
	return result.Call, ctx, &out, &errOut
}

// A heads-up whose deadline came first is not published; rewake retry
// publishes it, once, under the id the journal fixed.
func TestAHeadsUpPastItsDeadlineIsFinishedByRetry(t *testing.T) {
	dir, self, _ := toolSession(t)
	call, ctx, out, errOut := expiredCall(t, "send", "web", "late look", "--notify", "--wait", "0")
	if err := handleSend(ctx, call); err == nil {
		t.Fatalf("a call past its deadline succeeded: %s", out)
	}
	if letters := headsUps(t, dir, "late look"); len(letters) != 0 {
		t.Fatal("published after the deadline")
	}
	records := unresolved(t, dir, self.Epoch(), ctx.scope.digest)
	if len(records) != 1 || records[0].Phase != receipt.Open || records[0].Notify == nil || !strings.Contains(errOut.String(), "rewake retry "+records[0].Token) {
		t.Fatalf("records %+v, said %s", records, errOut)
	}
	pinned := records[0].Notify.MessageID
	for range 2 {
		if code, out, errOut := run("retry", records[0].Token); code == ExitFailed || code == ExitUsage {
			t.Fatalf("retry: %d %s %s", code, out, errOut)
		}
	}
	letters := headsUps(t, dir, "late look")
	if len(letters) != 1 || letters[0].ID != pinned {
		t.Fatalf("retries published %+v, want the one letter %s", letters, pinned)
	}
}

// A shell repeating the words of an operation still open does not run them
// again, however old that operation is: it cannot tell a lost call from a new
// one, and names the receipt that finishes it. A finished one stands in the
// way of nothing.
func TestTheShellRefersAnOpenOperationToItsReceipt(t *testing.T) {
	dir, self, _ := toolSession(t)
	call, ctx, _, _ := expiredCall(t, "send", "web", "lost look", "--notify", "--wait", "0")
	_ = handleSend(ctx, call)
	records := unresolved(t, dir, self.Epoch(), ctx.scope.digest)
	if len(records) != 1 {
		t.Fatalf("records %+v", records)
	}
	record := records[0]
	record.Created = time.Now().Add(-48 * time.Hour)
	if err := receipt.Save(dir, "api", record); err != nil {
		t.Fatal(err)
	}
	code, out, _ := run("send", "web", "lost look", "--notify", "--wait", "0")
	if code != ExitPending || !strings.Contains(out, "rewake retry "+record.Token) {
		t.Fatalf("a shell repeat of open words: %d %s", code, out)
	}
	if letters := headsUps(t, dir, "lost look"); len(letters) != 0 {
		t.Fatalf("the shell repeat published %d letters", len(letters))
	}
	if code, _, errOut := run("retry", record.Token); code == ExitFailed || code == ExitUsage {
		t.Fatalf("retry: %s", errOut)
	}
	if letters := headsUps(t, dir, "lost look"); len(letters) != 1 {
		t.Fatalf("published %d letters", len(letters))
	}
	// Finished now: the same words from the shell are a new heads-up.
	if code, out, errOut := run("send", "web", "lost look", "--notify", "--wait", "0"); code == ExitFailed || code == ExitUsage || strings.Contains(out, "not finished") {
		t.Fatalf("a shell send after the operation finished: %d %s %s", code, out, errOut)
	}
	if letters := headsUps(t, dir, "lost look"); len(letters) != 2 {
		t.Fatalf("published %d letters", len(letters))
	}
	// Shell calls alone are never joined by their words.
	for range 2 {
		if code, _, errOut := run("send", "web", "shell look", "--notify", "--wait", "0"); code == ExitFailed || code == ExitUsage {
			t.Fatalf("shell send: %s", errOut)
		}
	}
	if letters := headsUps(t, dir, "shell look"); len(letters) != 2 {
		t.Fatalf("two shell sends published %d letters", len(letters))
	}
}

// A mark repeated in one turn is the first mark: its time stays the first
// call's, so it cannot stretch into a later turn.
func TestAPendingMarkRepeatedInOneTurnIsOneMark(t *testing.T) {
	dir, self, web := toolSession(t)
	readFrom(t, dir, web)
	turnStarted(t, dir, self, markAt-1)
	tool := newToolCaller(t)
	first := tool.run("pending", "the suite is running")
	if first.code != ExitOK || !strings.Contains(first.out, "receipt") {
		t.Fatalf("first mark: %+v", first)
	}
	again := tool.run("pending", "the suite is running")
	if again.code != ExitOK || !strings.Contains(again.out, "ran earlier") {
		t.Fatalf("repeat: %+v", again)
	}
	called := tool.tickets[0].CalledBoot
	if text, held, err := markWithin(dir, "api", self.Epoch(), called-1, called); err != nil || !held || text != "the suite is running" {
		t.Fatalf("the mark does not carry the first call's time: %q %v %v", text, held, err)
	}
}

// A refusal is an answer like any other: the same words in the same turn get
// it again, and another turn asks anew.
func TestARefusedMarkIsReplayedInItsTurn(t *testing.T) {
	dir, self, web := toolSession(t)
	turnStarted(t, dir, self, markAt-1)
	tool := newToolCaller(t)
	if refused := tool.run("pending", "the suite is running"); refused.code != ExitUsage {
		t.Fatalf("a mark with nothing owed: %+v", refused)
	}
	readFrom(t, dir, web)
	if again := tool.run("pending", "the suite is running"); again.code != ExitUsage || !strings.Contains(again.out, "ran earlier") {
		t.Fatalf("the same words in the same turn: %+v", again)
	}
	tool.turn = "turn-2"
	if next := tool.run("pending", "the suite is running"); next.code != ExitOK || strings.Contains(next.out, "ran earlier") {
		t.Fatalf("the same words in the next turn: %+v", next)
	}
}

// unresolved lists api's unsettled operations with these words.
func unresolved(t *testing.T, dir, epoch, digest string) []receipt.Record {
	t.Helper()
	records, err := receipt.Unresolved(dir, "api", epoch, digest)
	if err != nil {
		t.Fatal(err)
	}
	return records
}
