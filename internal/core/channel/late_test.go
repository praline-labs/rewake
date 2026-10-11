package channel

import (
	"strings"
	"testing"
	"time"
)

// A ticket folded late ends the failures before it and no later one
// (docs/mail-bridge-channel.md, rule 5): the interval goes on from the first
// failure after it, however many came after that, and a shell success since
// then still counts.
func TestALateTicketKeepsTheFirstFailureAfterIt(t *testing.T) {
	r := New(true, "", stampAt(0))
	for _, at := range []time.Duration{1, 3, 5, 6} {
		r.Fold(Event{Kind: HelloRefused, Descendant: true, At: stampAt(at * time.Second)})
	}
	r.Fold(Event{Kind: ShellObserved, OK: true, At: stampAt(4 * time.Second)})
	r.Fold(Event{Kind: Validated, At: stampAt(2 * time.Second), Issued: stampAt(time.Second).Boot})
	if r.Interval != stampAt(3*time.Second) || r.Class != ClassServerRefused || r.Category() != CategoryShell {
		t.Fatalf("interval %v class %q category %s, want the failure at 3s, the latest class, shell", r.Interval, r.Class, r.Category())
	}
	// A second late ticket between the remaining failures moves it on again.
	r.Fold(Event{Kind: Validated, At: stampAt(5500 * time.Millisecond), Issued: stampAt(5 * time.Second).Boot})
	if r.Interval != stampAt(6*time.Second) || r.Category() != CategoryFailing {
		t.Fatalf("interval %v category %s, want the failure at 6s, failing", r.Interval, r.Category())
	}
}

// A ticket folded after a later close ends the failure before
// it, but the close stands: the server gone from the close, with a later
// hello noted as a reconnection — never working, and main is told of no
// recovery.
func TestALateTicketDoesNotUndoALaterClose(t *testing.T) {
	for _, tc := range []struct {
		name  string
		hello bool
	}{
		{name: "closed"},
		{name: "closed, then a hello", hello: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := New(true, "", stampAt(0))
			r.Fold(Event{Kind: HelloRefused, Descendant: true, At: stampAt(time.Second)})
			var notices Notices
			to := Recipient{Role: ToMain, Name: "main", Epoch: "e1"}
			told, ok := notices.Plan(&r, to, stampAt(time.Second))
			if !ok {
				t.Fatal("main was not told of the failure")
			}
			notices.Settle(told.Seq, Landed, stampAt(time.Second).Boot)
			r.Fold(Event{Kind: Hello, Generation: 1, At: stampAt(2 * time.Second)})
			r.Fold(Event{Kind: Closed, Generation: 1, Alive: true, At: stampAt(4 * time.Second)})
			if tc.hello {
				r.Fold(Event{Kind: Hello, Generation: 2, At: stampAt(5 * time.Second)})
			}
			r.Fold(Event{Kind: Validated, At: stampAt(3 * time.Second), Issued: stampAt(2 * time.Second).Boot})
			if r.Interval != stampAt(4*time.Second) || r.Class != ClassServerGone || r.Working() ||
				(r.Reconnected == stampAt(5*time.Second)) != tc.hello {
				t.Fatalf("%+v, want the server gone from the close at 4s", r)
			}
			if p, sent := notices.Plan(&r, to, stampAt(6*time.Second)); sent && strings.HasPrefix(p.Body, FirstMainWorks) {
				t.Fatalf("main told of a recovery the later close undid: %q", p.Body)
			}
			// The next ticket, after the close, is the recovery.
			r.Fold(Event{Kind: Validated, At: stampAt(7 * time.Second), Issued: stampAt(6 * time.Second).Boot})
			if !r.Working() {
				t.Fatalf("tool %q after a ticket later than the close", r.Tool)
			}
		})
	}
}
