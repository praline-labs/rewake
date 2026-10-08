package server_test

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/receipt"
)

// The evidence test (docs/mail-bridge-checks.md): a part counts as shown only
// when the harness recorded, as a success, the very answer the call printed,
// whole and within its bound. Every other record acknowledges nothing, and
// the true record afterwards still does.

// completeAs records a call's result as the harness might have kept it:
// content and isError as given.
func (r *rig) completeAs(c toolCall, content any, isError bool) {
	r.t.Helper()
	acknowledges := r.acknowledges(c)
	encoded, _ := json.Marshal(map[string]any{"method": "item/completed", "params": map[string]any{
		"threadId": "conversation", "turnId": c.turn,
		"item": map[string]any{
			"type": "mcpToolCall", "id": c.id, "server": "rewake", "tool": "rewake", "status": "completed",
			"arguments": map[string]any{"words": c.words}, "error": nil,
			"result": map[string]any{"content": content, "isError": isError},
		},
	}})
	r.endpoint.CodexEvent(encoded)
	if !acknowledges {
		return
	}
	select {
	case <-r.acknowledged:
	case <-time.After(10 * time.Second):
		r.t.Fatal("the acknowledgment did not run")
	}
}

// texts is a content list of text items.
func texts(items ...string) []map[string]string {
	content := make([]map[string]string, 0, len(items))
	for _, item := range items {
		content = append(content, map[string]string{"type": "text", "text": item})
	}
	return content
}

// answerOf is the content the server sent, item by item.
func answerOf(c toolCall) []string {
	items := make([]string, 0, len(c.result.Content))
	for _, item := range c.result.Content {
		items = append(items, item.Text)
	}
	return items
}

// reencoded is a JSON text with every non-ASCII rune escaped: the same value,
// other bytes.
func reencoded(text string) string {
	var out strings.Builder
	for _, r := range text {
		if r < 0x80 {
			out.WriteRune(r)
			continue
		}
		if r > 0xffff {
			hi, lo := 0xd800+(r-0x10000)>>10, 0xdc00+(r-0x10000)&0x3ff
			out.WriteString(`\u` + strconv.FormatInt(int64(hi), 16) + `\u` + strconv.FormatInt(int64(lo), 16))
			continue
		}
		out.WriteString(`\u` + strconv.FormatInt(int64(r)|0x10000, 16)[1:])
	}
	return out.String()
}

func TestOnlyTheRecordedAnswerItselfIsEvidence(t *testing.T) {
	escaped := "a \"quoted\" \\ line\nстрока — 🙂\ttab"
	for _, test := range []struct {
		name   string
		bodies []string
		words  []string
		// recorded is what the harness kept instead of the answer.
		recorded func(c toolCall, bodies []string) (any, bool)
	}{
		{
			"a body equal to a server line, recorded as that line",
			[]string{"Rewake: the answer of this call did not fit one tool result; run the same words in the shell.\n"},
			[]string{"inbox"},
			func(_ toolCall, bodies []string) (any, bool) { return texts(bodies[0]), false },
		},
		{
			"the same, marked an error",
			[]string{"Rewake: the answer of this call did not fit one tool result; run the same words in the shell.\n"},
			[]string{"inbox"},
			func(_ toolCall, bodies []string) (any, bool) { return texts(bodies[0]), true },
		},
		{
			"two letters with one body, the answer carrying it once",
			[]string{"one body for two letters", "one body for two letters"},
			[]string{"inbox"},
			func(c toolCall, bodies []string) (any, bool) {
				items := answerOf(c)
				items[0] = strings.Replace(items[0], bodies[0], "", 1)
				return texts(items...), false
			},
		},
		{
			"a replacement that kept the body",
			[]string{"the body a replacement kept"},
			[]string{"inbox"},
			func(_ toolCall, bodies []string) (any, bool) {
				return texts("[output replaced by the harness]\n" + bodies[0]), false
			},
		},
		{
			"a --json answer re-encoded",
			[]string{escaped},
			[]string{"inbox", "--json"},
			func(c toolCall, _ []string) (any, bool) {
				items := answerOf(c)
				items[0] = reencoded(items[0])
				return texts(items...), false
			},
		},
		{
			"a result kept as a preview",
			[]string{"the body of a previewed result"},
			[]string{"inbox"},
			func(c toolCall, _ []string) (any, bool) { return answerOf(c)[0][:40], false },
		},
		{
			"the whole answer, shown shortened past the bound",
			[]string{"the body of a shortened result"},
			[]string{"inbox"},
			func(c toolCall, _ []string) (any, bool) {
				return texts(append(answerOf(c), strings.Repeat("x", bridge.ResultCap))...), false
			},
		},
		{
			"the exact answer, marked an error",
			[]string{"the body of an error"},
			[]string{"inbox"},
			func(c toolCall, _ []string) (any, bool) { return texts(answerOf(c)...), true },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			r := newRig(t, bridge.CodexTransport)
			r.start()
			ids := make([]string, 0, len(test.bodies))
			for _, body := range test.bodies {
				ids = append(ids, r.letter(body))
			}
			r.nextTurn()
			c := r.call(test.words...)
			if c.ended || c.result.IsError {
				t.Fatalf("the read: %+v", c.result)
			}
			for _, body := range test.bodies {
				if !strings.Contains(c.result.text(), strings.Trim(mustJSON(t, body, test.words), `"`)) && !strings.Contains(c.result.text(), body) {
					t.Fatalf("the answer does not show %q: %s", body, c.result.text())
				}
			}
			content, isError := test.recorded(c, test.bodies)
			r.completeAs(c, content, isError)
			for _, id := range ids {
				if !r.unread(id) {
					t.Fatal("a record other than the answer read a letter")
				}
			}
			// The call's one result is spent; the true record of
			// the same call after it changes nothing.
			if err := r.complete(c, true); err != nil {
				t.Fatalf("the true record: %v", err)
			}
			for _, id := range ids {
				if !r.unread(id) {
					t.Fatal("a second record of one call read a letter")
				}
			}
			// The letters show again to the next read, whose true
			// record reads them: the forged one was the only reason
			// the first did not.
			r.nextTurn()
			again := r.call(test.words...)
			if err := r.complete(again, true); err != nil {
				t.Fatalf("the next read: %v", err)
			}
			for _, id := range ids {
				if r.unread(id) {
					t.Fatal("the true record of the next read did not read the letter")
				}
			}
		})
	}
}

// mustJSON is body as the answer carries it: encoded under --json.
func mustJSON(t *testing.T, body string, words []string) string {
	t.Helper()
	if len(words) < 2 || words[1] != "--json" {
		return body
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

// A completion whose binding cannot be read is spent all the same: the
// result recorded again starts nothing, the letter stays unread, and a new
// call that retries the operation reads it.
func TestACompletionWhoseBindingCannotBeReadIsSpent(t *testing.T) {
	t.Parallel()
	r := newRig(t, bridge.CodexTransport)
	r.start()
	id := r.letter("read only by a new call")
	r.nextTurn()
	c := r.call("inbox")
	if c.ended || c.result.IsError {
		t.Fatalf("the first call: %+v", c.result)
	}
	token, err := receipt.Bound(r.dir, "api", r.self.Epoch(), bridge.CallKey(r.transport, "conversation", c.id))
	if err != nil {
		t.Fatal(err)
	}
	plan := r.plan(&wrapperPlan{readFail: "/calls/"})
	r.endpoint.CodexEvent(codexEvent("item/completed", c.turn, c.id, c.words, c.result.Content))
	for until := time.Now().Add(time.Second); plan.failedReads() == 0; time.Sleep(time.Millisecond) {
		if time.Now().After(until) {
			t.Fatal("the binding was never read")
		}
	}
	r.plan(&wrapperPlan{})
	r.endpoint.CodexEvent(codexEvent("item/completed", c.turn, c.id, c.words, c.result.Content))
	select {
	case err := <-r.acknowledged:
		t.Fatalf("the result recorded again started an acknowledgment: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if !r.unread(id) {
		t.Fatal("a completion whose binding could not be read read the letter")
	}
	next := r.call("retry", token)
	if next.ended || next.result.IsError {
		t.Fatalf("the retry: %+v", next.result)
	}
	if err := r.complete(next, true); err != nil || r.unread(id) {
		t.Fatalf("the retry's read: %v, unread %v", err, r.unread(id))
	}
}
