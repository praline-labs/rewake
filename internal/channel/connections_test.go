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
func foldInEveryOrder(harness Harness, events []Event, check func(order []int, r Record)) {
	for _, order := range permutations(len(events)) {
		r := New(harness, true, "", stampAt(0))
		for _, index := range order {
			r.Fold(events[index])
		}
		check(order, r)
	}
}

// Two servers across a ticket: the newer closes, a startup failure comes
// while the older lives and is none, the older's close is the server gone
// on both harnesses, and the last failure is the class shown.
func TestTwoConnectionsAcrossATicketInEveryOrder(t *testing.T) {
	events := []Event{
		{Kind: Hello, Generation: 1, At: stampAt(time.Second)},
		{Kind: Hello, Generation: 2, At: stampAt(2 * time.Second)},
		{Kind: Validated, Issued: stampAt(2500 * time.Millisecond).Boot, At: stampAt(3 * time.Second)},
		{Kind: Closed, Generation: 2, Alive: true, At: stampAt(4 * time.Second)},
		{Kind: StartupFailed, At: stampAt(5 * time.Second)},
		{Kind: Closed, Generation: 1, Alive: true, At: stampAt(6 * time.Second)},
		{Kind: ShellObserved, OK: true, At: stampAt(7 * time.Second)},
		{Kind: NotObserved, At: stampAt(8 * time.Second)},
	}
	for _, harness := range []Harness{Codex, Claude} {
		interval, category := stampAt(6*time.Second), CategoryShell
		foldInEveryOrder(harness, events, func(order []int, r Record) {
			if r.Tool != ToolFailing || r.Interval != interval || r.Class != ClassNotObserved ||
				r.ClassAt != stampAt(8*time.Second) || r.Worked != stampAt(3*time.Second) ||
				r.Category() != category || len(r.Live) != 0 || r.Generation != 2 || r.Timer != 0 ||
				r.Reconnected.Boot != 0 || r.Shell == nil || !r.Shell.OK || r.Shell.At != stampAt(7*time.Second) {
				t.Fatalf("%s in order %v: %+v", harness, order, r)
			}
		})
	}
}

// A hello at the timer's end is in time and one a nanosecond later is not:
// the failure is stamped at the end, the hello noted as a reconnection.
func TestAHelloAtTheTimersEndIsInTime(t *testing.T) {
	for _, late := range []time.Duration{0, time.Nanosecond} {
		events := []Event{
			{Kind: ThreadAdmitted, At: stampAt(time.Second)},
			{Kind: TimerPassed, At: stampAt(30 * time.Second)},
			{Kind: Hello, Generation: 1, At: stampAt(16*time.Second + late)},
		}
		foldInEveryOrder(Codex, events, func(order []int, r Record) {
			if r.Open() != (late != 0) || r.Tool != ToolConnected || r.Timer != 0 {
				t.Fatalf("a hello %v past the end, in order %v: %+v", late, order, r)
			}
			if late != 0 && (r.Interval != stampAt(16*time.Second) || r.Class != ClassNoHello || r.Reconnected != stampAt(16*time.Second+late)) {
				t.Fatalf("a late hello, in order %v: %+v, want the failure at the timer's end", order, r)
			}
		})
	}
}

// A ticket stops the hello timer, and a thread after it starts its own.
func TestATicketStopsTheTimerAndTheNextThreadStartsOne(t *testing.T) {
	events := []Event{
		{Kind: ThreadAdmitted, At: stampAt(time.Second)},
		{Kind: TimerPassed, At: stampAt(50 * time.Second)},
		{Kind: Validated, Issued: stampAt(19 * time.Second).Boot, At: stampAt(20 * time.Second)},
		{Kind: ThreadAdmitted, At: stampAt(21 * time.Second)},
		{Kind: ShellObserved, OK: true, At: stampAt(37 * time.Second)},
	}
	foldInEveryOrder(Codex, events, func(order []int, r Record) {
		if r.Worked != stampAt(20*time.Second) || r.Interval != stampAt(36*time.Second) || r.Class != ClassNoHello ||
			r.ClassAt != stampAt(36*time.Second) || r.Category() != CategoryShell || r.Timer != 0 {
			t.Fatalf("in order %v: %+v, want only the second timer failing", order, r)
		}
	})
}

// A hello told late undoes the conditional failures it shows were none —
// the other connection's close and a startup failure — and its own close is
// then the server gone, at its own time.
func TestALateHelloUndoesFailuresUntilTheLastClose(t *testing.T) {
	r := New(Codex, true, "", stampAt(0))
	for _, e := range []Event{
		{Kind: Hello, Generation: 1, At: stampAt(time.Second)},
		{Kind: Closed, Generation: 1, Alive: true, At: stampAt(3 * time.Second)},
		{Kind: StartupFailed, At: stampAt(4 * time.Second)},
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
