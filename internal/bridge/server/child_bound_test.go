package server_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/bridge/endpoint"
	"github.com/praline-labs/rewake/internal/receipt"
)

// A child past its deadline (docs/mail-bridge-server.md#running-the-child):
// held after its confirmation, it is killed with its group five seconds past
// the deadline, and the call answers from its binding. A process the kill
// does not reach — here one of its own session holding the child's stdout,
// which to the server is a child that did not die — keeps the call's slot
// while the server serves: the server waits for it, and answers other calls
// meanwhile. Once stdin ended, before the kill or after it, the server waits
// for such a child only a second past the kill, answers its call as unknown
// and exits without it.
func TestAChildPastItsDeadline(t *testing.T) {
	t.Parallel()
	const span = time.Second
	const killed = span + 5*time.Second
	for _, lingers := range []bool{false, true} {
		for _, eof := range []string{"stdin open", "stdin ends before the kill", "stdin ends after the kill"} {
			name := "it dies by the kill, " + eof
			if lingers {
				name = "a process of it outlives the kill, " + eof
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				childPastItsDeadline(t, span, killed, lingers, eof)
			})
		}
	}
}

func childPastItsDeadline(t *testing.T, span, killed time.Duration, lingers bool, eof string) {
	r := newRig(t, bridge.CodexTransport, func(c *endpoint.Config) { c.Span = span })
	h := newHold(t)
	r.fault = h.specAt("child", "confirmed")
	if lingers {
		r.fault += ";child:orphan=12@" + string(h)
	}
	t.Cleanup(h.free)
	r.start()
	r.letter("a task from web")
	first := toolCall{id: "first", turn: r.nextTurn(), words: []string{"inbox"}}
	r.observe(first.turn, first.id, first.words)
	begun := time.Now()
	answered := make(chan toolCall, 1)
	go func() {
		first.result, first.ended = r.server.call(t, first.words, r.meta(first.turn, first.id))
		answered <- first
	}()
	h.reached(t)
	var closed time.Time
	closeStdin := func() {
		closed = time.Now()
		if err := r.server.in.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if eof == "stdin ends before the kill" {
		closeStdin()
	}
	if lingers && eof != "stdin ends before the kill" {
		// Past the kill, another call of the same server runs while the
		// first one's slot is still taken.
		time.Sleep(killed + time.Second)
		h.release(t)
		other := r.call("whoami")
		if other.ended || other.result.IsError {
			t.Fatalf("a call beside a lingering one: %+v", other.result)
		}
		select {
		case c := <-answered:
			t.Fatalf("the call answered while its output was still held: %+v", c.result)
		default:
		}
	}
	if eof == "stdin ends after the kill" {
		if !lingers {
			// The killed child's call answers by itself; stdin ends after.
			select {
			case c := <-answered:
				answered <- c
			case <-time.After(killed + 4*time.Second):
				t.Fatal("the killed child's call never answered")
			}
		}
		closeStdin()
	}
	var c toolCall
	select {
	case c = <-answered:
	case <-time.After(30 * time.Second):
		t.Fatal("the call never answered")
	}
	took, sinceEOF := time.Since(begun), time.Since(closed)
	unknown := lingers && eof != "stdin open"
	text := c.result.text()
	// How long the call took comes first: a server that waits for what
	// holds the output answers late, whatever it then says.
	switch {
	case !lingers && (took < killed-time.Second || took > killed+4*time.Second):
		t.Fatalf("the child was killed after %s", took)
	case lingers && eof == "stdin open" && took < 11*time.Second:
		t.Fatalf("the call answered after %s, before what held its output ended", took)
	case unknown && took < killed-time.Second:
		t.Fatalf("the call answered after %s, before its hard bound", took)
	case eof == "stdin ends before the kill" && lingers && sinceEOF > killed+3*time.Second:
		t.Fatalf("the call answered %s after its stdin ended, waiting for what held its output", sinceEOF)
	case eof == "stdin ends after the kill" && lingers && sinceEOF > 3*time.Second:
		t.Fatalf("the call answered %s after its stdin ended, its child already killed", sinceEOF)
	case c.ended || !c.result.IsError:
		t.Fatalf("the killed child's call answered: %+v", c.result)
	case !unknown && !strings.Contains(text, "before it began any operation"):
		t.Fatalf("the killed child's call answered: %s", text)
	case unknown && (!strings.Contains(text, "outlived its kill") || !strings.Contains(text, "outcome is unknown") || strings.Contains(text, "nothing ran")):
		t.Fatalf("a call whose child outlived its kill at the end of stdin answered: %s", text)
	}
	if eof == "stdin open" {
		return
	}
	// What held the output lives 12 s from the child's confirmation; the
	// server is gone well before it.
	select {
	case <-r.server.ended:
	case <-time.After(2 * time.Second):
		t.Fatalf("the server outlived its stdin by %s", time.Since(closed))
	}
	if lingers && time.Since(begun) > 11*time.Second {
		t.Fatalf("the server ended %s after the call, waiting for what held the output", time.Since(begun))
	}
}

// A read child that bound its operation before something of it held its
// output past the kill: at the end of stdin its call answers unknown with
// that operation's retry, the answer reads nothing, and the retry on a new
// server reads the letter.
func TestAnOutlivedCallWithABindingNamesItsRetry(t *testing.T) {
	t.Parallel()
	r := newRig(t, bridge.CodexTransport, func(c *endpoint.Config) { c.Span = time.Second })
	h := newHold(t)
	r.fault = "child:orphan=15@" + string(h)
	r.start()
	id := r.letter("not read by an unknown answer")
	c := toolCall{id: "bound-before-kill", turn: r.nextTurn(), words: []string{"inbox"}}
	r.observe(c.turn, c.id, c.words)
	_, waiter := r.server.send(t, "tools/call", map[string]any{"name": "rewake", "arguments": map[string]any{"words": c.words}, "_meta": r.meta(c.turn, c.id)})
	var token string
	for until := time.Now().Add(2 * time.Second); token == ""; time.Sleep(time.Millisecond) {
		if time.Now().After(until) {
			t.Fatal("the child never bound its operation")
		}
		token, _ = receipt.Bound(r.dir, "api", r.self.Epoch(), bridge.CallKey(r.transport, "conversation", c.id))
	}
	if err := r.server.in.Close(); err != nil {
		t.Fatal(err)
	}
	line, ok := r.server.wait(t, waiter)
	if !ok {
		t.Fatal("the server ended without the bound call's answer")
	}
	var answer struct {
		Result callResult `json:"result"`
	}
	if err := json.Unmarshal(line, &answer); err != nil {
		t.Fatal(err)
	}
	c.result = answer.Result
	if text := c.result.text(); !c.result.IsError || !strings.Contains(text, "outlived its kill") || !strings.Contains(text, "rewake retry "+token) || strings.Contains(text, "nothing ran") {
		t.Fatalf("the bound call answered: %s", text)
	}
	_ = r.complete(c, false)
	if !r.unread(id) {
		t.Fatal("an unknown answer read the letter")
	}
	r.fault = ""
	r.start()
	next := r.call("retry", token)
	if next.ended || next.result.IsError {
		t.Fatalf("the retry: %+v", next.result)
	}
	if err := r.complete(next, true); err != nil || r.unread(id) {
		t.Fatalf("the retry's read: %v, unread %v", err, r.unread(id))
	}
}
