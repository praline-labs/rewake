//go:build rewakefixture

package toolrig

import (
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/bridge/endpoint"
	"github.com/praline-labs/rewake/internal/harness/fixture"
	"github.com/praline-labs/rewake/internal/receipt"
)

// A child past its deadline (docs/mail-bridge-transport.md), rebuilt from
// bridge/server/child_bound_test.go: the server's end of stdin is the
// wrapper's endpoint closing. Held after its confirmation, the child is killed
// with its group five seconds past the deadline, and the call answers from its
// binding. A process the kill does not reach — here one of its own session
// holding the child's stdout, which to the endpoint is a child that did not
// die — keeps the call's slot while the endpoint serves: the endpoint waits
// for it, and answers other calls meanwhile. Once the endpoint closes, before
// the kill or after it, it waits for such a child only a second past the
// kill, answers its call as unknown, and Close returns without it.
func TestAChildPastItsDeadline(t *testing.T) {
	t.Parallel()
	const span = time.Second
	const killed = span + 5*time.Second
	for _, lingers := range []bool{false, true} {
		for _, eof := range []string{"the endpoint open", "it closes before the kill", "it closes after the kill"} {
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
	r := newRig(t, func(c *endpoint.Config) { c.Span = span })
	h := newHold(t)
	r.fault = h.specAt("child", "confirmed")
	if lingers {
		r.fault += ";child:orphan=12@" + string(h)
	}
	t.Cleanup(h.free)
	r.start()
	r.letter("a task from web")
	turn := r.nextTurn()
	r.observe(turn, "first", []string{"inbox"})
	begun := time.Now()
	answered := make(chan toolCall, 1)
	go func() {
		c := toolCall{id: "first", turn: turn, words: []string{"inbox"}}
		c.result, c.ended = r.request(turn, c.id, c.words)
		answered <- c
	}()
	h.reached(t)
	var closed time.Time
	closedDone := make(chan time.Time, 1)
	closeEndpoint := func() {
		closed = time.Now()
		go func() {
			r.endpoint.Close()
			closedDone <- time.Now()
		}()
	}
	if eof == "it closes before the kill" {
		closeEndpoint()
	}
	if lingers && eof != "it closes before the kill" {
		// Past the kill, another call runs while the first one's slot is
		// still taken.
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
	if eof == "it closes after the kill" {
		if !lingers {
			// The killed child's call answers by itself; the close is after.
			select {
			case c := <-answered:
				answered <- c
			case <-time.After(killed + 4*time.Second):
				t.Fatal("the killed child's call never answered")
			}
		}
		closeEndpoint()
	}
	var c toolCall
	select {
	case c = <-answered:
	case <-time.After(30 * time.Second):
		t.Fatal("the call never answered")
	}
	took, sinceClose := time.Since(begun), time.Since(closed)
	unknown := lingers && eof != "the endpoint open"
	text := c.result.text()
	// How long the call took comes first: an endpoint that waits for what
	// holds the output answers late, whatever it then says.
	switch {
	case !lingers && (took < killed-time.Second || took > killed+4*time.Second):
		t.Fatalf("the child was killed after %s", took)
	case lingers && eof == "the endpoint open" && took < 11*time.Second:
		t.Fatalf("the call answered after %s, before what held its output ended", took)
	case unknown && took < killed-time.Second:
		t.Fatalf("the call answered after %s, before its hard bound", took)
	case eof == "it closes before the kill" && lingers && sinceClose > killed+3*time.Second:
		t.Fatalf("the call answered %s after the endpoint began closing, waiting for what held its output", sinceClose)
	case eof == "it closes after the kill" && lingers && sinceClose > 3*time.Second:
		t.Fatalf("the call answered %s after the endpoint began closing, its child already killed", sinceClose)
	case c.ended || !c.result.IsError:
		t.Fatalf("the killed child's call answered: %+v", c.result)
	case !unknown && !strings.Contains(text, "before it began any operation"):
		t.Fatalf("the killed child's call answered: %s", text)
	case unknown && (!strings.Contains(text, "outlived its kill") || !strings.Contains(text, "outcome is unknown") || strings.Contains(text, "nothing ran")):
		t.Fatalf("a call whose child outlived its kill as the endpoint closed answered: %s", text)
	}
	if eof == "the endpoint open" {
		return
	}
	// What held the output lives 12 s from the child's start; Close
	// returns well before it.
	select {
	case <-closedDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("Close outlived the call's answer by %s", time.Since(closed))
	}
	if lingers && time.Since(begun) > 11*time.Second {
		t.Fatalf("Close returned %s after the call, waiting for what held the output", time.Since(begun))
	}
}

// A read child that bound its operation before something of it held its
// output past the kill: as the endpoint closes, its call answers unknown with
// that operation's retry, the answer reads nothing, and the retry — in the
// shell, since the wrapper is gone — reads the letter.
func TestAnOutlivedCallWithABindingNamesItsRetry(t *testing.T) {
	t.Parallel()
	r := newRig(t, func(c *endpoint.Config) { c.Span = time.Second })
	h := newHold(t)
	r.fault = "child:orphan=15@" + string(h)
	r.start()
	id := r.letter("not read by an unknown answer")
	turn := r.nextTurn()
	c := toolCall{id: "bound-before-kill", turn: turn, words: []string{"inbox"}}
	r.observe(c.turn, c.id, c.words)
	answered := make(chan toolCall, 1)
	go func() {
		c.result, c.ended = r.request(c.turn, c.id, c.words)
		answered <- c
	}()
	var token string
	for until := time.Now().Add(2 * time.Second); token == ""; time.Sleep(time.Millisecond) {
		if time.Now().After(until) {
			t.Fatal("the child never bound its operation")
		}
		token, _ = receipt.Bound(r.dir, "api", r.self.Epoch(), bridge.CallKey(fixture.Transport, thread, c.id))
	}
	go r.endpoint.Close()
	select {
	case c = <-answered:
	case <-time.After(30 * time.Second):
		t.Fatal("the bound call never answered")
	}
	if text := c.result.text(); c.ended || !c.result.IsError || !strings.Contains(text, "outlived its kill") || !strings.Contains(text, "rewake retry "+token) || strings.Contains(text, "nothing ran") {
		t.Fatalf("the bound call answered: %+v", c.result)
	}
	// The wrapper is gone: no result the program reports reaches it.
	if !r.unread(id) {
		t.Fatal("an unknown answer read the letter")
	}
	r.fault = ""
	if out, err := r.shell("retry", token); err != nil || !strings.Contains(out, "not read by an unknown answer") {
		t.Fatalf("the retry in the shell: %v: %s", err, out)
	}
	if r.unread(id) {
		t.Fatal("the retry's read left the letter unread")
	}
}
