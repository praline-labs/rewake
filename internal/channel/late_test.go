package channel

import (
	"slices"
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

// A's server gone and A resumed: a second server seen and closed before the
// answer is the start the admission waited for, whether its binding to A is
// known or not, and folded in either order its close is the failure shown,
// never the timer's end (docs/mail-bridge-channel-codex.md#as-built).
func TestAReselectedConversationsHelloEndsTheExpectedStartInEitherOrder(t *testing.T) {
	for _, bound := range []bool{false, true} {
		events := append(selectionPrefix(),
			Event{Kind: Closed, Generation: 1, Alive: true, At: stampAt(6 * s)},
			Event{Kind: SelectionAdmitted, Thread: "A", At: stampAt(10 * s)},
			Event{Kind: Hello, Generation: 2, At: stampAt(11 * s)})
		if bound {
			events = append(events, Event{Kind: Bound, Generation: 2, Thread: "A", At: stampAt(12 * s)})
		}
		events = append(events,
			Event{Kind: Closed, Generation: 2, Alive: true, At: stampAt(13 * s)},
			Event{Kind: Selected, Thread: "A", At: stampAt(14 * s)},
			Event{Kind: TimerPassed, At: stampAt(26 * s)})
		for _, arrival := range []string{"in event order", "in reverse"} {
			r := New(Codex, true, "", stampAt(0))
			arranged := slices.Clone(events)
			if arrival == "in reverse" {
				slices.Reverse(arranged)
			}
			for _, e := range arranged {
				r.Fold(e)
			}
			if r.Class != ClassServerGone || r.ClassAt != stampAt(13*s) {
				t.Errorf("bound %v, %s: class %q at %v, want the server gone at 13s", bound, arrival, r.Class, r.ClassAt)
			}
		}
	}
}
