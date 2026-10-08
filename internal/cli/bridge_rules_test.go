package cli

import (
	"context"
	"encoding/json"
	"fmt"
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

// The rules of docs/mail-bridge-cli.md#the-rules-the-code-holds, each against
// the defect the first acceptance found breaking it.

// Rule 1. A heads-up published to a run that ended before the journal said so
// is not taken for undone: the same words in the turn replay that answer, and
// nothing goes to the session that took the name since.
func TestAnUncertainPublicationIsNotDiscarded(t *testing.T) {
	dir, self, web := toolSession(t)
	words := []string{"send", "web", "crash publication", "--notify", "--wait", "0"}
	call, ctx, _, _ := expiredCall(t, words...)
	_ = handleSend(ctx, call)
	records := unresolved(t, dir, self.Epoch(), ctx.scope.digest)
	if len(records) != 1 {
		t.Fatal(records)
	}
	record := records[0]
	// A crash after publication, before the journal recorded it.
	letter := inbox.Message{ID: record.Notify.MessageID, From: "api", FromEpoch: self.Epoch(), To: "web", ToEpoch: web.Epoch(), Kind: inbox.Note, Text: "crash publication", CreatedAt: time.Now()}
	if _, err := inbox.PublishOnce(context.Background(), dir, letter, nil); err != nil {
		t.Fatal(err)
	}
	replacement := otherRun(t, dir, "web")
	tool := newToolCaller(t)
	if first := tool.run(words...); first.code != ExitFailed || !strings.Contains(first.errOut, "has since ended") {
		t.Fatalf("the replacement was not refused: %+v", first)
	}
	if again := tool.run(words...); !strings.Contains(again.out, "ran earlier") {
		t.Fatalf("the same words again: %+v", again)
	}
	for _, message := range headsUps(t, dir, "crash publication") {
		if message.ToEpoch == replacement.Epoch() {
			t.Fatal("the heads-up went to the replacement")
		}
	}
}

// Rule 1. When the run that ended no longer shows either way, the answer
// says the effect is unknown and the record stays with its run.
func TestAHeadsUpToAnEndedRunWithNothingLeftStaysUnknown(t *testing.T) {
	dir, self, _ := toolSession(t)
	words := []string{"send", "web", "lost to an ended run", "--notify", "--wait", "0"}
	call, ctx, _, _ := expiredCall(t, words...)
	_ = handleSend(ctx, call)
	records := unresolved(t, dir, self.Epoch(), ctx.scope.digest)
	if len(records) != 1 {
		t.Fatal(records)
	}
	otherRun(t, dir, "web")
	tool := newToolCaller(t)
	if first := tool.run(words...); first.code != ExitFailed || !strings.Contains(first.errOut, "no longer known") {
		t.Fatalf("an unknown effect: %+v", first)
	}
	record, err := receipt.Load(dir, "api", self.Epoch(), records[0].Token)
	if err != nil || !record.Uncertain || record.Phase != receipt.Done {
		t.Fatalf("record %+v, %v", record, err)
	}
	receipt.Sweep(dir, "api", self.Epoch(), time.Now().Add(time.Hour), func(string) bool { return false })
	if _, err := receipt.Load(dir, "api", self.Epoch(), record.Token); err != nil {
		t.Fatalf("the sweep removed a record whose effect is unknown: %v", err)
	}
	// Rule 6: finished or not, an effect nobody can tell stops the shell's
	// same words, which would otherwise go to the session that took the name.
	if code, out, errOut := run(words...); code != ExitFailed || !strings.Contains(errOut, "rewake retry "+record.Token) {
		t.Fatalf("a shell repeat of an unknown effect: %d %s %s", code, out, errOut)
	}
	if letters := headsUps(t, dir, "lost to an ended run"); len(letters) != 0 {
		t.Fatalf("published %d letters", len(letters))
	}
}

// Rule 1. Age decides nothing: a shell repeating the words of an operation
// open for eleven minutes does not publish beside it, and the retry of that
// operation publishes the one letter.
func TestAnOpenOperationIsNotBypassedByAge(t *testing.T) {
	dir, self, _ := toolSession(t)
	words := []string{"send", "web", "lost old look", "--notify", "--wait", "0"}
	call, ctx, _, _ := expiredCall(t, words...)
	_ = handleSend(ctx, call)
	records := unresolved(t, dir, self.Epoch(), ctx.scope.digest)
	if len(records) != 1 {
		t.Fatal(records)
	}
	record := records[0]
	record.Created = time.Now().Add(-11 * time.Minute)
	if err := receipt.Save(dir, "api", record); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := run(words...); code == ExitFailed || code == ExitUsage {
		t.Fatal(errOut)
	}
	_, _, _ = run("retry", record.Token)
	if got := len(headsUps(t, dir, "lost old look")); got != 1 {
		t.Fatalf("the shell and the retry published %d letters", got)
	}
}

// Rule 3. A read whose deadline passed shows and claims nothing.
func TestAnExpiredReadShowsAndClaimsNothing(t *testing.T) {
	dir, self, web := toolSession(t)
	id := rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": self.Epoch(), "kind": "task", "text": longText})
	call, ctx, out, _ := expiredCall(t, "inbox")
	err := handleInbox(ctx, call)
	if _, claimed := inbox.ReadClaimed(dir, "api", id); err == nil || claimed || out.Len() != 0 {
		t.Fatalf("an expired read: err=%v claimed=%v, %d bytes shown", err, claimed, out.Len())
	}
}

// Rule 3. The deadline is checked once the mailbox lock is held: a wait for
// it that outlasts the call marks nothing, and neither does a call that
// expired before.
func TestAPendingMarkIsNotMadeAfterItsDeadline(t *testing.T) {
	eachDeclaration(t, func(t *testing.T, reused bool) {
		dir, self, web := toolSession(t)
		readFrom(t, dir, web)
		turnStarted(t, dir, self, markAt-1)
		call, ctx, out, _ := expiredCall(t, "pending", "still waiting")
		ctx.scope.ticket.CalledBoot = boottime.Now()
		ctx.scope.ticket.DeadlineBoot = boottime.Now() + int64(100*time.Millisecond)
		locked, unlocked := make(chan struct{}), make(chan struct{})
		go func() {
			defer close(unlocked)
			_ = state.WithMailboxLock(context.Background(), dir, "api", func() error {
				close(locked)
				time.Sleep(300 * time.Millisecond)
				return nil
			})
		}()
		<-locked
		err := handlePending(ctx, call)
		<-unlocked
		if err == nil {
			t.Fatalf("pending succeeded after its deadline: %s", out)
		}

		call, ctx, _, errOut := expiredCall(t, "pending", "expired already")
		if err := handlePending(ctx, call); err == nil {
			t.Fatal("an expired mark succeeded")
		}
		called := ctx.scope.ticket.CalledBoot
		if _, held, _ := markWithin(dir, "api", self.Epoch(), called-1, called); held {
			t.Fatal("an expired call marked the turn")
		}
		records := unresolved(t, dir, self.Epoch(), ctx.scope.digest)
		if len(records) != 1 || !strings.Contains(errOut.String(), "rewake retry "+records[0].Token) {
			t.Fatalf("records %+v, said %s", records, errOut)
		}
		// Finished from the same turn: a retry from the shell could not show it
		// runs in that turn, and would not mark (pending_turn.go); nor can a call
		// whose transport does not declare its turn ids never reused.
		tool := newToolCaller(t)
		tool.reused = reused
		retried := tool.run("retry", records[0].Token)
		_, held, _ := markWithin(dir, "api", self.Epoch(), called-1, called)
		if reused {
			if retried.code != ExitFailed || !strings.Contains(retried.errOut, unproven) || held {
				t.Fatalf("a retry that cannot show its turn: %d %s, marked %v", retried.code, retried.errOut, held)
			}
			return
		}
		if retried.code != ExitOK {
			t.Fatalf("retry: %d %s", retried.code, retried.errOut)
		}
		if text, held, _ := markWithin(dir, "api", self.Epoch(), called-1, called); !held || text != "expired already" {
			t.Fatalf("the retry did not mark at the call's time: %q %v", text, held)
		}
	})
}

// Rule 3. A continuation looks at a letter before any part of it is first
// shown, whichever part it names: one withdrawn meanwhile is its tombstone.
func TestAContinuationRefreshesALetterBeforeAnyPart(t *testing.T) {
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
	got := tool.run("inbox", "--next", token+".1.1")
	if got.code != ExitOK || strings.Contains(got.out, "WITHDRAWN_SECRET") || !strings.Contains(got.out, "withdrew") {
		t.Fatalf("a continuation to a later part of a withdrawn letter: %+v", got)
	}
}

// Rule 3. A retry through the tool is held to the tool's surface: a shell's
// heads-up with its text from stdin is not the tool's to run.
func TestAToolRetryKeepsToTheToolSurface(t *testing.T) {
	dir, self, _ := toolSession(t)
	words := []string{"send", "web", "-", "--notify", "--wait", "0"}
	record, _, err := receipt.Begin(dir, "api", receipt.Key{Epoch: self.Epoch(), Digest: "shell"}, receipt.Record{Words: words, Transport: receipt.Shell})
	if err != nil {
		t.Fatal(err)
	}
	withStdin(t, "TEXT_FROM_STDIN")
	tool := newToolCaller(t)
	got := tool.run("retry", record.Token)
	if len(headsUps(t, dir, "TEXT_FROM_STDIN")) > 0 || got.code != ExitFailed || !strings.Contains(got.errOut, "in the shell") {
		t.Fatalf("a tool retry of stdin words: %+v", got)
	}
}

// Rule 3. A shell retry sends the text the first call fixed, and does not
// read its input again.
func TestAShellRetryDoesNotReadStdinAgain(t *testing.T) {
	dir, self, web := toolSession(t)
	words := []string{"send", "web", "-", "--notify", "--wait", "0"}
	record, _, err := receipt.Begin(dir, "api", receipt.Key{Epoch: self.Epoch(), Digest: "shell"}, receipt.Record{
		Words: words, Transport: receipt.Shell,
		Notify: &receipt.NotifyStep{MessageID: inbox.NewID(), To: "web", ToEpoch: web.Epoch(), Text: "the first input"},
	})
	if err != nil {
		t.Fatal(err)
	}
	withStdin(t, "a later input")
	if code, _, errOut := run("retry", record.Token); code == ExitFailed || code == ExitUsage {
		t.Fatalf("retry: %d %s", code, errOut)
	}
	if len(headsUps(t, dir, "the first input")) != 1 || len(headsUps(t, dir, "a later input")) != 0 {
		t.Fatal("the retry did not send the fixed text")
	}
}

func withStdin(t *testing.T, text string) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fmt.Fprint(writer, text)
	_ = writer.Close()
	previous := os.Stdin
	os.Stdin = reader
	t.Cleanup(func() { os.Stdin = previous; _ = reader.Close() })
}

// Rule 4. A task a tool read began, the shell read whole, and the report
// settled, owes nothing again when a late acknowledgment comes after the
// sweep removed its status and copy.
func TestALateAcknowledgmentOwesNothingTwice(t *testing.T) {
	dir, self, web := toolSession(t)
	id := rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": self.Epoch(), "kind": "task", "text": "task"})
	tool := newToolCaller(t)
	first := tool.run("inbox", "--json")
	var part readPartModel
	if err := json.Unmarshal([]byte(first.out), &part); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := run("inbox"); code != ExitOK {
		t.Fatal(errOut)
	}
	for _, waiter := range inbox.Waiters(dir, "api", self.Epoch()) {
		if err := inbox.ClearAwaiting(dir, "api", self.Epoch(), waiter); err != nil {
			t.Fatal(err)
		}
	}
	// What the finished-mail sweep does a day later.
	if err := os.Remove(filepath.Join(state.InboxPath(dir, "api"), id+".status")); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(filepath.Join(state.DonePath(dir, "api"), id+".json"))
	if err := AcknowledgeRead(dir, "api", self.Epoch(), part.Receipt, whole(first), nil); err != nil {
		t.Fatal(err)
	}
	if waiters := inbox.Waiters(dir, "api", self.Epoch()); len(waiters) != 0 {
		t.Fatalf("a settled task is owed again: %+v", waiters)
	}
}

// Rule 4. A letter frozen and not shown yet that was read elsewhere in the
// meantime shows as no longer unread, and its text is not shown again.
func TestALetterReadElsewhereShowsAsGone(t *testing.T) {
	dir, self, web := toolSession(t)
	rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": self.Epoch(), "kind": "note", "text": longText})
	time.Sleep(2 * time.Millisecond)
	rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": self.Epoch(), "kind": "task", "text": "the short secret"})
	tool := newToolCaller(t)
	first := tool.run("inbox")
	if code, _, errOut := run("inbox"); code != ExitOK {
		t.Fatal(errOut)
	}
	var shown strings.Builder
	for _, answer := range readAll(t, tool, nextFrom(first.out)...) {
		shown.WriteString(answer.out)
	}
	if strings.Contains(shown.String(), "the short secret") || !strings.Contains(shown.String(), "no longer unread") {
		t.Fatalf("the rest of the read: %s", shown.String())
	}
}

// Rule 5. A diagnostic is bounded by its encoded size, escapes counted.
func TestAnEscapedDiagnosticFits(t *testing.T) {
	toolSession(t)
	_, ctx, _, _ := expiredCall(t, "inbox", "--owed")
	out, errOut := boundedAnswer(ctx, strings.Repeat("a", 7000), strings.Repeat("\x00", 1024))
	// Cut and kept, not given up on: the last resort also fits, and says
	// nothing.
	if !bridge.Fits(out, errOut) || out == "" || !strings.Contains(errOut, "cut here") {
		t.Fatalf("encoded size %d: %.80q", bridge.EncodedSize(out, errOut), errOut)
	}
	tool := newToolCaller(t)
	got := tool.run("inbox", "--"+strings.Repeat("<", 1000))
	if got.code != ExitUsage || !bridge.Fits(got.out, got.errOut) || !strings.Contains(got.errOut, "cut here") {
		t.Fatalf("a parser refusal: %d, encoded %d: %.80q", got.code, bridge.EncodedSize(got.out, got.errOut), got.errOut)
	}
}

// Rule 5. An answer that could not be kept for continuing still leaves inside
// the bound, naming the shell.
func TestAnAnswerThatCannotBeKeptStillFits(t *testing.T) {
	out, errOut := boundedAnswer(&Context{}, strings.Repeat("a", 7000), strings.Repeat("\x00", 900))
	if !bridge.Fits(out, errOut) || !strings.Contains(errOut, "shell") {
		t.Fatalf("encoded size %d: %q", bridge.EncodedSize(out, errOut), errOut)
	}
}

// Rule 5. Every text part names its letter, its part and count, and its byte
// range, a whole short letter as much as a slice of a long one.
func TestEveryTextPartNamesItself(t *testing.T) {
	dir, self, web := toolSession(t)
	id := rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": self.Epoch(), "kind": "task", "text": "short"})
	tool := newToolCaller(t)
	got := tool.run("inbox")
	if want := "Rewake: " + id + ", part 1 of 1, bytes 0–5 of 5."; !strings.Contains(got.out, want) {
		t.Fatalf("want %q in %s", want, got.out)
	}
}
