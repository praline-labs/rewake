package channel

import (
	"testing"
	"time"
)

// Connections, the timer and tickets in every arrival order, each against a
// state written out by hand rather than against another fold: the order and
// space tests compare folds with folds, and a rule wrong in both would pass
// them.

// foldInEveryOrder folds the events in each of their arrival orders and
// hands every resulting record to check.
func foldInEveryOrder(events []Event, check func(order []int, r Record)) {
	for _, order := range permutations(len(events)) {
		r := New(true, "", stampAt(0))
		for _, index := range order {
			r.Fold(events[index])
		}
		check(order, r)
	}
}

// Two transports across a ticket: the newer closes, a refused hello from the
// harness's tree comes while the older lives and is none, the older's close
// is the transport gone, and the last failure is the class shown.
func TestTwoConnectionsAcrossATicketInEveryOrder(t *testing.T) {
	events := []Event{
		{Kind: Hello, Generation: 1, At: stampAt(time.Second)},
		{Kind: Hello, Generation: 2, At: stampAt(2 * time.Second)},
		{Kind: Validated, Issued: stampAt(2500 * time.Millisecond).Boot, At: stampAt(3 * time.Second)},
		{Kind: Closed, Generation: 2, Alive: true, At: stampAt(4 * time.Second)},
		{Kind: HelloRefused, Descendant: true, At: stampAt(5 * time.Second)},
		{Kind: Closed, Generation: 1, Alive: true, At: stampAt(6 * time.Second)},
		{Kind: ShellObserved, OK: true, At: stampAt(7 * time.Second)},
		{Kind: HelloRefused, Descendant: true, At: stampAt(8 * time.Second)},
	}
	foldInEveryOrder(events, func(order []int, r Record) {
		if r.Tool != ToolFailing || r.Interval != stampAt(6*time.Second) || r.Class != ClassServerRefused ||
			r.ClassAt != stampAt(8*time.Second) || r.Worked != stampAt(3*time.Second) ||
			r.Category() != CategoryShell || len(r.Live) != 0 || r.Generation != 2 || r.Timer != 0 ||
			r.Reconnected.Boot != 0 || r.Shell == nil || !r.Shell.OK || r.Shell.At != stampAt(7*time.Second) {
			t.Fatalf("in order %v: %+v", order, r)
		}
	})
}

// A hello at the timer's end is in time and one a nanosecond later is not:
// the failure is stamped at the end, the hello noted as a reconnection.
func TestAHelloAtTheTimersEndIsInTime(t *testing.T) {
	for _, late := range []time.Duration{0, time.Nanosecond} {
		events := []Event{
			{Kind: TimerPassed, At: stampAt(30 * time.Second)},
			{Kind: Hello, Generation: 1, At: stampAt(HelloAtStart + late)},
		}
		foldInEveryOrder(events, func(order []int, r Record) {
			if r.Open() != (late != 0) || r.Tool != ToolConnected || r.Timer != 0 {
				t.Fatalf("a hello %v past the end, in order %v: %+v", late, order, r)
			}
			if late != 0 && (r.Interval != stampAt(HelloAtStart) || r.Class != ClassNoHello || r.Reconnected != stampAt(HelloAtStart+late)) {
				t.Fatalf("a late hello, in order %v: %+v, want the failure at the timer's end", order, r)
			}
		})
	}
}

// A ticket before the timer's end stops the timer: a clock long past the
// end then fails nothing.
func TestATicketStopsTheTimer(t *testing.T) {
	events := []Event{
		{Kind: TimerPassed, At: stampAt(50 * time.Second)},
		{Kind: Validated, Issued: stampAt(9 * time.Second).Boot, At: stampAt(10 * time.Second)},
		{Kind: ShellObserved, OK: true, At: stampAt(37 * time.Second)},
	}
	foldInEveryOrder(events, func(order []int, r Record) {
		if r.Worked != stampAt(10*time.Second) || r.Open() || r.Category() != CategoryTool || r.Timer != 0 {
			t.Fatalf("in order %v: %+v, want the tool working and no failure", order, r)
		}
	})
}

// A hello told late undoes the conditional failures it shows were none —
// the other connection's close and a refused hello from the harness's tree —
// and its own close is then the transport gone, at its own time.
func TestALateHelloUndoesFailuresUntilTheLastClose(t *testing.T) {
	r := New(true, "", stampAt(0))
	for _, e := range []Event{
		{Kind: Hello, Generation: 1, At: stampAt(time.Second)},
		{Kind: Closed, Generation: 1, Alive: true, At: stampAt(3 * time.Second)},
		{Kind: HelloRefused, Descendant: true, At: stampAt(4 * time.Second)},
	} {
		r.Fold(e)
	}
	if !r.Open() || r.Interval != stampAt(3*time.Second) {
		t.Fatalf("before the late hello: %+v", r)
	}
	r.Fold(Event{Kind: Hello, Generation: 2, At: stampAt(2 * time.Second)})
	if r.Open() || r.Tool != ToolConnected || len(r.Live) != 1 || r.Live[0] != 2 {
		t.Fatalf("after the late hello: %+v, want no failure and the second connection live", r)
	}
	r.Fold(Event{Kind: Closed, Generation: 2, Alive: true, At: stampAt(7 * time.Second)})
	if r.Interval != stampAt(7*time.Second) || r.Class != ClassServerGone || len(r.Live) != 0 {
		t.Fatalf("after the last close: %+v, want the server gone at 7s", r)
	}
}
