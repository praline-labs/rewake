package channel

import (
	"cmp"
	"reflect"
	"slices"
	"testing"
)

// The space's oracles over a whole path: where its interval may start, which
// times its failures may carry, what folding it in another arrival order
// leaves, and when two records are the same.

// intervalAfterTickets says whether an interval starts at a failure of the
// path later than every ticket of the path.
func intervalAfterTickets(folded []Event, interval Stamp) bool {
	var worked int64
	for _, e := range folded {
		if e.Kind == Validated {
			worked = max(worked, e.At.Boot)
		}
	}
	return interval.Boot > worked && slices.Contains(failureTimes(folded, true), interval.Boot)
}

// failureTimes are the times a failure of the path may carry: each failure
// event's own — a close's only when closes says so — and the end of the
// hello timer from the launch, once a later timer or transport event of the
// path showed it had gone by.
func failureTimes(folded []Event, closes bool) []int64 {
	var clock int64
	for _, e := range folded {
		switch e.Kind {
		case Denied, ShellObserved, Validated, Exited:
		default:
			if e.Kind != HelloRefused || e.Descendant {
				clock = max(clock, e.At.Boot)
			}
		}
	}
	var times []int64
	if end := stampAt(HelloAtStart).Boot; end <= clock {
		times = append(times, end)
	}
	for _, e := range folded {
		if failure(e.Kind) && e.Kind != TimerPassed && (closes || e.Kind != Closed) {
			times = append(times, e.At.Boot)
		}
	}
	return times
}

// The arrival orders a path is folded in again: by event time, and in the
// reverse of the walk's.
func sortedByTime(events []Event) {
	slices.SortStableFunc(events, func(a, b Event) int { return cmp.Compare(a.At.Boot, b.At.Boot) })
}

func reversed(events []Event) { slices.Reverse(events) }

// foldedAs is the record a path's events leave folded in another arrival
// order, from the same launch at stampAt(0); the exit, which ends what is
// of the run, stays last.
func foldedAs(folded []Event, r Record, arrange func([]Event)) Record {
	again := New(r.Tool != ToolNone, r.Reason, stampAt(0))
	var exit []Event
	events := slices.DeleteFunc(slices.Clone(folded), func(e Event) bool {
		if e.Kind == Exited {
			exit = append(exit, e)
			return true
		}
		return false
	})
	arrange(events)
	for _, e := range append(events, exit...) {
		again.Fold(e)
	}
	return again
}

// equal compares what a record shows and keeps: every exported field.
func equal(a, b Record) bool {
	shells := a.Shell == nil && b.Shell == nil ||
		a.Shell != nil && b.Shell != nil && a.Shell.OK == b.Shell.OK && a.Shell.Class == b.Shell.Class && same(a.Shell.At, b.Shell.At)
	return shells && a.Tool == b.Tool && a.Class == b.Class && same(a.ClassAt, b.ClassAt) &&
		a.Reason == b.Reason && same(a.Interval, b.Interval) && same(a.Worked, b.Worked) && same(a.Reconnected, b.Reconnected) &&
		slices.Equal(a.Live, b.Live) && a.Generation == b.Generation && a.Timer == b.Timer &&
		same(a.Block, b.Block) && a.Issued == b.Issued && a.Frozen == b.Frozen
}

func same(a, b Stamp) bool { return a.Boot == b.Boot && a.Wall.Equal(b.Wall) }

// equal is written out field by field for speed; a field added to the
// record without it would be compared by nothing.
func TestEqualComparesEveryField(t *testing.T) {
	if n := reflect.TypeFor[Record]().NumField(); n != 15 {
		t.Fatalf("the record has %d fields; equal compares 14 and the history", n)
	}
}
