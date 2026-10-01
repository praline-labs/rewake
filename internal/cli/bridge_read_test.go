package cli

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// longText is a letter several results long, with characters that take more
// than a byte and escapes that grow under JSON.
var longText = strings.Repeat("данные и \"кавычки\" — ещё строка\n", 220)

var receiptLine = regexp.MustCompile(`receipt ([0-9a-f]{24})`)

// readAll follows a read under the tool from its first words to its end.
func readAll(t *testing.T, tool *toolCaller, words ...string) []toolRun {
	t.Helper()
	var answers []toolRun
	for step := 0; words != nil; step++ {
		if step > 40 {
			t.Fatal("the read does not end")
		}
		answer := tool.run(words...)
		if answer.code != ExitOK || !bridge.Fits(answer.out, answer.errOut) {
			t.Fatalf("step %d: %d, %d bytes: %s", step, answer.code, bridge.EncodedSize(answer.out, answer.errOut), answer.errOut)
		}
		answers = append(answers, answer)
		words = nextFrom(answer.out)
		var part readPartModel
		if json.Unmarshal([]byte(answer.out), &part) == nil && part.NextWords != nil {
			words = part.NextWords
		}
	}
	return answers
}

func whole(answer toolRun) bridge.Exposure {
	return bridge.Exposure{CallID: answer.callID, Direct: true, Succeeded: true, ResultBytes: bridge.EncodedSize(answer.out, answer.errOut)}
}

// A letter read through the tool is read once every part of it reached the
// model whole, and not before: printing is not reading.
func TestAToolReadIsReadOnlyWhenEveryPartArrived(t *testing.T) {
	dir, self, web := toolSession(t)
	id := rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": self.Epoch(), "kind": "task", "text": longText})
	tool := newToolCaller(t)
	answers := readAll(t, tool, "inbox", "--json")

	var text strings.Builder
	var token string
	for _, answer := range answers {
		var part readPartModel
		if err := json.Unmarshal([]byte(answer.out), &part); err != nil {
			t.Fatalf("a part is no whole envelope: %v: %s", err, answer.out)
		}
		token = part.Receipt
		for _, letter := range part.Letters {
			if letter.End-letter.Start > bridge.BodyCap || letter.Total != len(longText) {
				t.Fatalf("part %+v", letter)
			}
			text.WriteString(letter.Text)
		}
	}
	if len(answers) < 3 || text.String() != longText {
		t.Fatalf("%d bytes came in %d parts as %d bytes", len(longText), len(answers), text.Len())
	}
	if !unreadIDs(t, dir, self)[id] {
		t.Fatal("printing the parts read the letter")
	}
	if _, claimed := inbox.ReadClaimed(dir, "api", id); !claimed {
		t.Fatal("a letter shown in part is not claimed")
	}

	shortened := whole(answers[0])
	shortened.Shortened = true
	if err := AcknowledgeRead(dir, "api", self.Epoch(), token, shortened); !errors.Is(err, ErrNotWhole) {
		t.Fatalf("a shortened result acknowledged: %v", err)
	}
	for _, answer := range answers[:len(answers)-1] {
		if err := AcknowledgeRead(dir, "api", self.Epoch(), token, whole(answer)); err != nil {
			t.Fatal(err)
		}
	}
	if !unreadIDs(t, dir, self)[id] || len(inbox.Waiters(dir, "api", self.Epoch())) != 0 {
		t.Fatal("a letter with a part not arrived was read")
	}
	last := answers[len(answers)-1]
	for range 2 {
		if err := AcknowledgeRead(dir, "api", self.Epoch(), token, whole(last)); err != nil {
			t.Fatal(err)
		}
	}
	if unreadIDs(t, dir, self)[id] {
		t.Fatal("the letter is unread with every part arrived")
	}
	if _, claimed := inbox.ReadClaimed(dir, "api", id); claimed {
		t.Fatal("the claim outlived the read")
	}
	if waiters := inbox.Waiters(dir, "api", self.Epoch()); len(waiters) != 1 || len(waiters[0].Messages) != 1 {
		t.Fatalf("the task is owed %+v, want once", waiters)
	}
	if err := AcknowledgeRead(dir, "api", "another-run", token, whole(last)); err == nil {
		t.Fatal("another run acknowledged this one's read")
	}
}

// The same words in the same turn are the same read: its letters again, and
// not the ones that came since. Another turn reads anew.
func TestARepeatedToolReadInOneTurnIsTheSameRead(t *testing.T) {
	dir, self, web := toolSession(t)
	for _, text := range []string{"first note", "second note"} {
		rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": self.Epoch(), "kind": "note", "text": text})
		time.Sleep(2 * time.Millisecond)
	}
	tool := newToolCaller(t)
	first := tool.run("inbox")
	if first.code != ExitOK || !strings.Contains(first.out, "first note") || !strings.Contains(first.out, "second note") || strings.Contains(first.out, "ran earlier") {
		t.Fatalf("first read: %+v", first)
	}
	rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": self.Epoch(), "kind": "note", "text": "third note"})
	again := tool.run("inbox")
	if again.code != ExitOK || !strings.Contains(again.out, "ran earlier") || !strings.Contains(again.out, "second note") || strings.Contains(again.out, "third note") {
		t.Fatalf("repeat in the turn: %+v", again)
	}
	if receiptLine.FindString(first.out) != receiptLine.FindString(again.out) {
		t.Fatalf("the repeat has another receipt: %q, %q", first.out, again.out)
	}
	tool.turn = "turn-2"
	if next := tool.run("inbox"); !strings.Contains(next.out, "third note") || strings.Contains(next.out, "ran earlier") {
		t.Fatalf("the next turn: %+v", next)
	}
}

// A read that found nothing answers the same nothing to the same words for
// the rest of the turn, naming how to reach what came since; the next turn
// reads anew.
func TestAnEmptyToolReadIsTheSameNothingInItsTurn(t *testing.T) {
	dir, self, web := toolSession(t)
	tool := newToolCaller(t)
	if first := tool.run("inbox"); first.code != ExitOK || !strings.Contains(first.out, "no new messages") {
		t.Fatalf("an empty read: %+v", first)
	}
	rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": self.Epoch(), "kind": "note", "text": "came later"})
	again := tool.run("inbox")
	if again.code != ExitOK || strings.Contains(again.out, "came later") || !strings.Contains(again.out, "--peek") {
		t.Fatalf("the same words in the turn: %+v", again)
	}
	tool.turn = "turn-2"
	if next := tool.run("inbox"); !strings.Contains(next.out, "came later") {
		t.Fatalf("the next turn: %+v", next)
	}
}

// A read the tool lost is finished from the shell, where printed is read as
// it always was.
func TestTheShellFinishesALostToolRead(t *testing.T) {
	dir, self, web := toolSession(t)
	id := rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": self.Epoch(), "kind": "task", "text": longText})
	tool := newToolCaller(t)
	first := tool.run("inbox")
	if first.code != ExitOK || nextFrom(first.out) == nil {
		t.Fatalf("first part: %+v", first)
	}
	token, _, _ := strings.Cut(nextFrom(first.out)[2], ".")
	// The shell's continuation alone leaves the tool's part unconfirmed.
	for words := nextFrom(first.out); words != nil; {
		code, out, errOut := run(words...)
		if code != ExitOK {
			t.Fatalf("shell continuation: %d %s", code, errOut)
		}
		words = nextFrom(out)
	}
	if !unreadIDs(t, dir, self)[id] {
		t.Fatal("a part only the tool showed counted as read")
	}
	code, out, errOut := run("retry", token)
	if code != ExitOK || !strings.Contains(out, "from web") {
		t.Fatalf("retry: %d %s %s", code, out, errOut)
	}
	for words := nextFrom(out); words != nil; {
		code, out, errOut := run(words...)
		if code != ExitOK {
			t.Fatalf("shell continuation: %d %s", code, errOut)
		}
		words = nextFrom(out)
	}
	if unreadIDs(t, dir, self)[id] {
		t.Fatal("the shell read every part and the letter is still unread")
	}
}

// A letter shown in part cannot be taken back; one the read froze but has not
// shown yet can, and the reader then finds its tombstone — whether it rides
// along after another letter or starts a response of its own.
func TestAWithdrawalMeetsAToolRead(t *testing.T) {
	dir, self, web := toolSession(t)
	for _, text := range []string{longText, "the secret plan", "another secret\n" + longText} {
		rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": self.Epoch(), "kind": "task", "text": text})
		time.Sleep(2 * time.Millisecond)
	}
	tool := newToolCaller(t)
	first := tool.run("inbox")
	if first.code != ExitOK || strings.Contains(first.out, "secret") {
		t.Fatalf("first part: %+v", first)
	}
	if err := withdrawLocked(dir, letterWith(t, dir, self, longText)); !errors.Is(err, inbox.ErrReadInProgress) {
		t.Fatalf("withdrawing a letter shown in part: %v", err)
	}
	refusal := withdrawRefusal(dir, letterWith(t, dir, self, longText), inbox.ErrReadInProgress, false)
	if !strings.Contains(refusal.Error(), "in parts") {
		t.Fatalf("refusal: %v", refusal)
	}
	if err := withdrawLocked(dir, letterWith(t, dir, self, "the secret plan")); err != nil {
		t.Fatalf("withdrawing a letter not shown yet: %v", err)
	}
	// Read up to the response the third letter starts, and withdraw it just
	// before: the response before it already looked at it.
	var joined strings.Builder
	words := nextFrom(first.out)
	for !strings.HasSuffix(words[2], ".2.0") {
		answer := tool.run(words...)
		joined.WriteString(answer.out)
		if words = nextFrom(answer.out); words == nil {
			t.Fatalf("the read ended before its third letter: %s", joined.String())
		}
	}
	if err := withdrawLocked(dir, letterWith(t, dir, self, "another secret\n"+longText)); err != nil {
		t.Fatalf("withdrawing a letter not shown yet: %v", err)
	}
	for _, answer := range readAll(t, tool, words...) {
		joined.WriteString(answer.out)
	}
	if strings.Contains(joined.String(), "secret") || strings.Count(joined.String(), "web withdrew its task") != 2 {
		t.Fatalf("after the withdrawals the read shows: %s", joined.String())
	}
}

func withdrawLocked(dir string, message inbox.Message) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return state.WithMailboxLock(ctx, dir, message.To, func() error {
		_, err := inbox.Withdraw(dir, message, nil)
		return err
	})
}

// A pending mark knows who waits on a letter still being read: its
// confirmation comes before the turn ends.
func TestPendingCountsALetterBeingRead(t *testing.T) {
	dir, self, web := toolSession(t)
	rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": self.Epoch(), "kind": "task", "text": longText})
	turnStarted(t, dir, self, markAt-1)
	tool := newToolCaller(t)
	if first := tool.run("inbox"); first.code != ExitOK {
		t.Fatalf("read: %+v", first)
	}
	answer := tool.run("pending", "the suite is running")
	if answer.code != ExitOK || !strings.Contains(answer.out, "web") {
		t.Fatalf("pending on a letter in progress: %+v", answer)
	}
}

// letterWith is the unread letter holding this text.
func letterWith(t *testing.T, dir string, self registry.Session, text string) inbox.Message {
	t.Helper()
	messages, _ := inbox.PeekUnread(dir, "api", self.Epoch())
	for _, message := range messages {
		if message.Text == text {
			return message
		}
	}
	t.Fatalf("no unread letter holds %.20q", text)
	return inbox.Message{}
}
