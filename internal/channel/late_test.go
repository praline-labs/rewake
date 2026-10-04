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
	r := New(Codex, true, "", stampAt(0))
	r.Fold(Event{Kind: CannotStart, At: stampAt(1 * time.Second)})
	r.Fold(Event{Kind: NotObserved, At: stampAt(3 * time.Second)})
	r.Fold(Event{Kind: CannotStart, At: stampAt(5 * time.Second)})
	r.Fold(Event{Kind: NotObserved, At: stampAt(6 * time.Second)})
	r.Fold(Event{Kind: ShellObserved, OK: true, At: stampAt(4 * time.Second)})
	r.Fold(Event{Kind: Validated, At: stampAt(2 * time.Second), Issued: stampAt(time.Second).Boot})
	if r.Interval != stampAt(3*time.Second) || r.Class != ClassNotObserved || r.Category() != CategoryShell {
		t.Fatalf("interval %v class %q category %s, want the failure at 3s, the latest class, shell", r.Interval, r.Class, r.Category())
	}
	// A second late ticket between the remaining failures moves it on again.
	r.Fold(Event{Kind: Validated, At: stampAt(5500 * time.Millisecond), Issued: stampAt(5 * time.Second).Boot})
	if r.Interval != stampAt(6*time.Second) || r.Category() != CategoryFailing {
		t.Fatalf("interval %v category %s, want the failure at 6s, failing", r.Interval, r.Category())
	}
}

// A ticket folded after a later close on Claude Code ends the failure before
// it, but the close stands: not connected, or connected again after a later
// hello — never working, and main is told of no recovery.
func TestALateTicketDoesNotUndoALaterClose(t *testing.T) {
	for _, tc := range []struct {
		name  string
		hello bool
		want  string
	}{
		{name: "closed", want: ToolNotConnected},
		{name: "closed, then a hello", hello: true, want: ToolConnected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := New(Claude, true, "", stampAt(0))
			r.Fold(Event{Kind: CannotStart, At: stampAt(time.Second)})
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
			if r.Open() || r.Tool != tc.want || r.Working() {
				t.Fatalf("open %v tool %q working %v, want the interval ended and %q", r.Open(), r.Tool, r.Working(), tc.want)
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
