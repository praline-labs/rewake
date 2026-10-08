//go:build rewakefixture

package toolrig

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/harness/fixture"
	"github.com/praline-labs/rewake/internal/receipt"
)

// The evidence test, rebuilt from bridge/server/evidence_test.go on the
// fixture's transport: a part counts as shown only when the harness recorded,
// as a success, the very answer the call printed, whole and within its bound.
// Every other record acknowledges nothing, and the true record afterwards
// still does not: a call's one result is spent.

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

// recorded is what the program reports in place of the answer.
type recorded struct {
	texts     []string
	isError   bool
	shortened bool
}

func TestOnlyTheRecordedAnswerItselfIsEvidence(t *testing.T) {
	escaped := "a \"quoted\" \\ line\nстрока — 🙂\ttab"
	for _, test := range []struct {
		name   string
		bodies []string
		words  []string
		record func(c toolCall, bodies []string) recorded
	}{
		{
			"a body equal to an endpoint line, recorded as that line",
			[]string{"Rewake: the answer of this call did not fit one tool result; run the same words in the shell.\n"},
			[]string{"inbox"},
			func(_ toolCall, bodies []string) recorded { return recorded{texts: []string{bodies[0]}} },
		},
		{
			"the same, marked an error",
			[]string{"Rewake: the answer of this call did not fit one tool result; run the same words in the shell.\n"},
			[]string{"inbox"},
			func(_ toolCall, bodies []string) recorded { return recorded{texts: []string{bodies[0]}, isError: true} },
		},
		{
			"two letters with one body, the answer carrying it once",
			[]string{"one body for two letters", "one body for two letters"},
			[]string{"inbox"},
			func(c toolCall, bodies []string) recorded {
				texts := append([]string(nil), c.result.Texts...)
				texts[0] = strings.Replace(texts[0], bodies[0], "", 1)
				return recorded{texts: texts}
			},
		},
		{
			"a replacement that kept the body",
			[]string{"the body a replacement kept"},
			[]string{"inbox"},
			func(_ toolCall, bodies []string) recorded {
				return recorded{texts: []string{"[output replaced by the harness]\n" + bodies[0]}}
			},
		},
		{
			"a --json answer re-encoded",
			[]string{escaped},
			[]string{"inbox", "--json"},
			func(c toolCall, _ []string) recorded {
				texts := append([]string(nil), c.result.Texts...)
				texts[0] = reencoded(texts[0])
				return recorded{texts: texts}
			},
		},
		{
			"a result kept as a preview",
			[]string{"the body of a previewed result"},
			[]string{"inbox"},
			func(c toolCall, _ []string) recorded { return recorded{texts: []string{c.result.Texts[0][:40]}} },
		},
		{
			"the whole answer, said to be shortened",
			[]string{"the body of a result shortened"},
			[]string{"inbox"},
			func(c toolCall, _ []string) recorded { return recorded{texts: c.result.Texts, shortened: true} },
		},
		{
			"the whole answer, shown past the bound",
			[]string{"the body of a result past the bound"},
			[]string{"inbox"},
			func(c toolCall, _ []string) recorded {
				return recorded{texts: append(append([]string(nil), c.result.Texts...), strings.Repeat("x", bridge.ResultCap))}
			},
		},
		{
			"the exact answer, marked an error",
			[]string{"the body of an error"},
			[]string{"inbox"},
			func(c toolCall, _ []string) recorded { return recorded{texts: c.result.Texts, isError: true} },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			r := newRig(t)
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
			kept := test.record(c, test.bodies)
			_ = r.report(c, command{Op: "result", Call: c.id, Texts: kept.texts, IsError: kept.isError, Shortened: kept.shortened})
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

// A result whose binding cannot be read is spent all the same: the result
// reported again starts nothing, the letter stays unread, and a new call that
// retries the operation reads it.
func TestACompletionWhoseBindingCannotBeReadIsSpent(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.start()
	id := r.letter("read only by a new call")
	r.nextTurn()
	c := r.call("inbox")
	if c.ended || c.result.IsError {
		t.Fatalf("the first call: %+v", c.result)
	}
	token, err := receipt.Bound(r.dir, "api", r.self.Epoch(), bridge.CallKey(fixture.Transport, thread, c.id))
	if err != nil {
		t.Fatal(err)
	}
	plan := r.plan(&wrapperPlan{readFail: "/calls/"})
	r.completed[c.id] = true
	if answer, err := r.ask(command{Op: "result", Call: c.id, Texts: c.result.Texts}); err != nil || answer.Error != "" {
		t.Fatalf("the result: %v %s", err, answer.Error)
	}
	for until := time.Now().Add(time.Second); plan.failedReads() == 0; time.Sleep(time.Millisecond) {
		if time.Now().After(until) {
			t.Fatal("the binding was never read")
		}
	}
	r.plan(&wrapperPlan{})
	if answer, err := r.ask(command{Op: "result", Call: c.id, Texts: c.result.Texts}); err != nil || answer.Error != "" {
		t.Fatalf("the result again: %v %s", err, answer.Error)
	}
	select {
	case err := <-r.acknowledged:
		t.Fatalf("the result reported again started an acknowledgment: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if !r.unread(id) {
		t.Fatal("a result whose binding could not be read read the letter")
	}
	next := r.call("retry", token)
	if next.ended || next.result.IsError {
		t.Fatalf("the retry: %+v", next.result)
	}
	if err := r.complete(next, true); err != nil || r.unread(id) {
		t.Fatalf("the retry's read: %v, unread %v", err, r.unread(id))
	}
}
