//go:build rewakefixture

package toolrig

import (
	"regexp"
	"strings"
	"testing"
)

// The path of a call, rebuilt from bridge/server/flow_test.go on the
// fixture's transport.

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
	r := newRig(t)
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
	r := newRig(t)
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
	r := newRig(t)
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
	r := newRig(t)
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

// A call the program asks to run without having reported it, or under ids
// other than those it reported, runs nothing.
func TestACallWithoutItsIdsRunsNothing(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.start()
	id := r.letter("the letter of a call without ids")
	turn := r.nextTurn()
	r.observe(turn, "call-seen", []string{"inbox"})
	for _, asked := range []command{
		{Op: "request", Turn: turn, Call: "call-never-seen", Tool: "inbox"},
		{Op: "request", Turn: turn, Tool: "inbox"},
		{Op: "request", Turn: "turn-other", Call: "call-seen", Tool: "inbox"},
		{Op: "request", Call: "call-seen", Tool: "inbox"},
	} {
		answer, ended := r.requestWith(asked)
		if ended || !answer.IsError || !strings.Contains(strings.Join(answer.Texts, ""), "nothing ran") {
			t.Fatalf("%+v: %+v", asked, answer)
		}
	}
	if !r.unread(id) {
		t.Fatal("a call without its ids showed the letter")
	}
}

// A call is bound only inside a turn the program was seen to start and not
// yet seen to end (T7): a call in a turn never started, or in one that has
// ended, runs nothing.
func TestACallOutsideAnOpenTurnRunsNothing(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.start()
	id := r.letter("the letter of a call outside its turn")
	r.observe("turn-never-started", "early", []string{"inbox"})
	if early, ended := r.request("turn-never-started", "early", []string{"inbox"}); ended || !early.IsError || !strings.Contains(early.text(), "nothing ran") {
		t.Fatalf("a call in a turn never started: %+v", early)
	}
	turn := r.nextTurn()
	r.endTurn()
	r.observe(turn, "late", []string{"inbox"})
	if late, ended := r.request(turn, "late", []string{"inbox"}); ended || !late.IsError || !strings.Contains(late.text(), "nothing ran") {
		t.Fatalf("a call in a turn that ended: %+v", late)
	}
	if !r.unread(id) {
		t.Fatal("a call outside an open turn showed the letter")
	}
}

// Words off the surface are refused by the endpoint — a tool not offered,
// an argument its schema does not name, words the CLI refuses — before any
// ticket, so nothing runs.
func TestWordsOffTheSurfaceAreRefused(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.start()
	r.nextTurn()
	for _, words := range [][]string{{"claude"}, {"send", "--file", "x", "web", "text"}, {"inbox", "--next", "not-a-token"}} {
		c := r.call(words...)
		if c.ended || !c.result.IsError || !strings.Contains(c.result.text(), "nothing ran") && !strings.Contains(c.result.text(), "exit status 2") {
			t.Fatalf("%v through the tool: %+v", words, c.result)
		}
	}
	if headsUps(r) != 0 {
		t.Fatal("a refused send published")
	}
}

// A transport that cannot show the result it handed the model refuses a read
// before its first effect and leaves the letter unread, while the rest of the
// tool still answers (T6).
func TestAToolThatReadsOffLeavesTheLetterUnread(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.switches = []string{unprovenEnv + "=1"}
	r.start()
	id := r.letter("the letter of a run that reads off")
	r.nextTurn()
	read := r.call("inbox")
	if !read.result.IsError || !strings.Contains(read.result.text(), "cannot show the result") || !r.unread(id) {
		t.Fatalf("a read: %+v", read.result)
	}
	if err := r.complete(read, true); err == nil && !r.unread(id) {
		t.Fatal("a refused read was acknowledged")
	}
	if !r.unread(id) {
		t.Fatal("a transport without the proof read the letter")
	}
	if who := r.call("whoami"); who.result.IsError {
		t.Fatalf("whoami: %+v", who.result)
	}
}
