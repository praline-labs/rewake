package server_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/inbox"
)

// unread says whether api's letter id still waits.
func (r *rig) unread(id string) bool {
	r.t.Helper()
	messages, err := inbox.PeekUnread(r.dir, "api", r.self.Epoch())
	if err != nil {
		r.t.Fatal(err)
	}
	for _, message := range messages {
		if message.ID == id {
			return true
		}
	}
	return false
}

var nextWords = regexp.MustCompile(`rewake (inbox --next \S+)`)

// next is the words an answer names for its continuation.
func next(answer string) []string {
	found := nextWords.FindStringSubmatch(answer)
	if found == nil {
		return nil
	}
	return strings.Fields(found[1])
}

// A letter read through the tool is read once the harness recorded the
// call's whole answer, not when the child printed it.
func TestAToolReadIsReadOnceItsAnswerArrived(t *testing.T) {
	t.Parallel()
	r := newRig(t, bridge.CodexTransport)
	r.start()
	id := r.letter("the letter of the first scenario")
	r.nextTurn()
	c := r.call("inbox")
	if c.ended || c.result.IsError || !strings.Contains(c.result.text(), "the letter of the first scenario") {
		t.Fatalf("the answer: %+v", c.result)
	}
	if !r.unread(id) {
		t.Fatal("printing the letter read it")
	}
	if err := r.complete(c, true); err != nil {
		t.Fatalf("the acknowledgment: %v", err)
	}
	if r.unread(id) {
		t.Fatal("the answer arrived and the letter is still unread")
	}
}

// A letter longer than one result is read part by part, each part a call of
// its own, and is read once the last part arrived.
func TestALongReadGoesOnThroughItsNextWords(t *testing.T) {
	t.Parallel()
	r := newRig(t, bridge.CodexTransport)
	r.start()
	text := strings.Repeat("данные и \"кавычки\" — строка\n", 300)
	id := r.letter(text)
	r.nextTurn()
	words := []string{"inbox"}
	parts := 0
	for ; words != nil; parts++ {
		if parts > 40 {
			t.Fatal("the read does not end")
		}
		c := r.call(words...)
		if c.ended || c.result.IsError {
			t.Fatalf("part %d: %+v", parts, c.result)
		}
		if err := r.complete(c, true); err != nil {
			t.Fatalf("part %d: %v", parts, err)
		}
		words = next(c.result.text())
		if words != nil && !r.unread(id) {
			t.Fatalf("the letter was read after part %d of more", parts)
		}
	}
	if parts < 3 || r.unread(id) {
		t.Fatalf("%d parts; unread after the last: %v", parts, r.unread(id))
	}
}

// A result the harness did not record as a success reads nothing, and the
// same words again in the same turn show the letter again.
func TestAFailedResultReadsNothing(t *testing.T) {
	t.Parallel()
	r := newRig(t, bridge.CodexTransport)
	r.start()
	id := r.letter("the letter of a failed result")
	r.nextTurn()
	first := r.call("inbox")
	if err := r.complete(first, false); err == nil {
		t.Fatal("a failed result was acknowledged")
	}
	if !r.unread(id) {
		t.Fatal("a failed result read the letter")
	}
	again := r.call("inbox")
	if !strings.Contains(again.result.text(), "the letter of a failed result") {
		t.Fatalf("the same words again: %+v", again.result)
	}
	if err := r.complete(again, true); err != nil || r.unread(id) {
		t.Fatalf("the second answer: %v, unread %v", err, r.unread(id))
	}
}

// A result recorded after its turn's end was noted reads nothing: the end's
// boundary is taken, and the acknowledgment arrives too late to write.
func TestAnAnswerAfterTheEndReadsNothing(t *testing.T) {
	t.Parallel()
	r := newRig(t, bridge.CodexTransport)
	r.start()
	id := r.letter("the letter of a late answer")
	r.nextTurn()
	c := r.call("inbox")
	r.endTurn()
	if err := r.complete(c, true); err == nil {
		t.Fatal("an answer after the end was acknowledged")
	}
	if !r.unread(id) {
		t.Fatal("an answer after the end read the letter")
	}
}

// A call that carries no native ids, or another transport's, runs nothing.
func TestACallWithoutItsIdsRunsNothing(t *testing.T) {
	t.Parallel()
	r := newRig(t, bridge.CodexTransport)
	r.start()
	id := r.letter("the letter of a call without ids")
	r.nextTurn()
	for _, meta := range []map[string]any{
		nil,
		{"claudecode/toolUseId": "call-x"},
		{"callId": "call-y", "claudecode/toolUseId": "call-y"},
	} {
		result, ended := r.server.call(t, []string{"inbox"}, meta)
		if ended || !result.IsError || !strings.Contains(result.text(), "nothing ran") {
			t.Fatalf("%v: %+v", meta, result)
		}
	}
	if !r.unread(id) {
		t.Fatal("a call without its ids showed the letter")
	}
}

// Words off the surface are refused by the server with the CLI's own
// refusal, before any ticket.
func TestWordsOffTheSurfaceAreRefused(t *testing.T) {
	t.Parallel()
	r := newRig(t, bridge.CodexTransport)
	r.start()
	r.nextTurn()
	c := r.call("claude")
	if !c.result.IsError || !strings.Contains(c.result.text(), "exit status 2") {
		t.Fatalf("a launch through the tool: %+v", c.result)
	}
}

// On Claude Code a read runs in the shell for now; the rest of the surface
// runs through the tool.
func TestAClaudeCallRunsThroughTheHooksObservation(t *testing.T) {
	t.Parallel()
	r := newRig(t, bridge.ClaudeTransport)
	r.start()
	id := r.letter("the letter of a Claude Code run")
	r.nextTurn()
	read := r.call("inbox")
	if !read.result.IsError || !strings.Contains(read.result.text(), "in the shell") || !r.unread(id) {
		t.Fatalf("a read: %+v", read.result)
	}
	who := r.call("whoami")
	if who.result.IsError || !strings.Contains(who.result.text(), "api") {
		t.Fatalf("whoami: %+v", who.result)
	}
}
