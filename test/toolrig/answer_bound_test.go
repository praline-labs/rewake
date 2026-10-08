//go:build rewakefixture

package toolrig

import (
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/harness/fixture"
	"github.com/praline-labs/rewake/internal/receipt"
)

// A transport that stops reading, rebuilt from
// bridge/server/stdout_bound_test.go. The old rig's server wrote every answer
// into one stdout a client could stop draining; here each call's answer goes
// to its own connection, and an answer is at most bridge.ResultCap, which a
// socket's buffer holds whole: the endpoint's write never waits on the
// reader. What the old test guarded stays: an answer read late lands whole,
// an answer nobody reads is no read and keeps nothing waiting, and the
// effect of a call whose answer never left is found again, not repeated.

// exchange is the endpoint's write deadline on an answer; a reader late past
// it still gets the answer whole.
const exchange = 5 * time.Second

func TestATransportThatStopsReading(t *testing.T) {
	t.Parallel()
	for _, leg := range []string{"it reads late", "it never reads, the endpoint open", "it never reads, the endpoint closes"} {
		t.Run(leg, func(t *testing.T) {
			t.Parallel()
			r := newRig(t)
			r.start()
			id := r.letter("the letter of a transport that stops reading")
			turn := r.nextTurn()
			c := toolCall{id: "unread-answer", turn: turn, words: []string{"inbox"}}
			r.observe(turn, c.id, c.words)
			tool, arguments := toolOf(c.words)
			asked := command{Op: "request", Turn: turn, Call: c.id, Tool: tool, Arguments: arguments}
			if leg == "it reads late" {
				asked.ReadAfter = exchange + time.Second
				answer, ended := r.requestWith(asked)
				c.result = callResult{Texts: answer.Texts, IsError: answer.IsError}
				if ended || c.result.IsError || !strings.Contains(c.result.text(), "the letter of a transport that stops reading") {
					t.Fatalf("an answer read late: %+v", answer)
				}
				if err := r.complete(c, true); err != nil || r.unread(id) {
					t.Fatalf("the late answer's read: %v, unread %v", err, r.unread(id))
				}
				return
			}
			asked.Unread = true
			if answer, ended := r.requestWith(asked); ended || answer.Error != "" {
				t.Fatalf("the request: %+v", answer)
			}
			// The call runs to its end and its answer is written to nobody.
			for until := time.Now().Add(10 * time.Second); ; time.Sleep(10 * time.Millisecond) {
				if _, err := receipt.Bound(r.dir, "api", r.self.Epoch(), bridge.CallKey(fixture.Transport, thread, c.id)); err == nil {
					break
				}
				if time.Now().After(until) {
					t.Fatal("the call never ran")
				}
			}
			if strings.HasSuffix(leg, "closes") {
				since := time.Now()
				r.endpoint.Close()
				if took := time.Since(since); took > 3*time.Second {
					t.Fatalf("the endpoint closed %s after, waiting on an answer nobody reads", took)
				}
			}
			if !r.unread(id) {
				t.Fatal("an answer nobody read read the letter")
			}
			if strings.HasSuffix(leg, "open") {
				next := r.call("inbox")
				if next.ended || !strings.Contains(next.result.text(), "the letter of a transport that stops reading") {
					t.Fatalf("a later call: %+v", next.result)
				}
				if err := r.complete(next, true); err != nil || r.unread(id) {
					t.Fatalf("the later call's read: %v, unread %v", err, r.unread(id))
				}
			}
		})
	}
}

// An effect whose answer never left is found again, not repeated: a send
// that published its heads-up, the harness's process gone before the
// endpoint answered, is joined by the same words in the same turn and by its
// retry from the harness restarted, and the heads-up stays one.
func TestALostAnswerOfAnEffectIsFoundAgain(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.start()
	r.nextTurn()
	r.plan(&wrapperPlan{killAt: "answer", kill: r.killProgram})
	words := []string{"send", "--notify", "--wait", "0", "web", faultHeadsUp}
	c := r.call(words...)
	if !c.ended {
		t.Fatalf("the send was answered: %+v", c.result)
	}
	if n := headsUps(r); n != 1 {
		t.Fatalf("the send published %d heads-ups", n)
	}
	token, err := receipt.Bound(r.dir, "api", r.self.Epoch(), bridge.CallKey(fixture.Transport, thread, c.id))
	if err != nil {
		t.Fatal(err)
	}
	r.start()
	for _, again := range [][]string{words, {"retry", token}} {
		next := r.call(again...)
		if next.ended || !strings.Contains(next.result.text(), "pending for web") {
			t.Fatalf("%v answered: %s", again, next.result.text())
		}
		if n := headsUps(r); n != 1 {
			t.Fatalf("%v left %d heads-ups", again, n)
		}
	}
}
