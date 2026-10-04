package channel

import (
	"cmp"
	"reflect"
	"slices"
	"testing"
	"time"
)

// The space's oracles over a whole path: where its interval may start, which
// times its failures may carry, what folding it in another arrival order
// leaves, and when two records are the same.

// intervalAfterTickets says whether an interval starts at a failure of the
// path later than every ticket of the path, and no later than the first such
// failure that fails whatever came before it.
func intervalAfterTickets(folded []Event, harness Harness, interval Stamp) bool {
	var worked int64
	for _, e := range folded {
		if e.Kind == Validated {
			worked = max(worked, e.At.Boot)
		}
	}
	for _, e := range folded {
		if unconditional(e.Kind) && e.At.Boot > worked && e.At.Boot < interval.Boot {
			return false
		}
	}
	return interval.Boot > worked && slices.Contains(failureTimes(folded, harness, true), interval.Boot)
}

// failureTimes are the times a failure of the path may carry: each failure
// event's own — a close's only when closes says so — and the end of each
// hello timer that a later timer or transport event of the path showed had
// gone by.
func failureTimes(folded []Event, harness Harness, closes bool) []int64 {
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
	for _, e := range folded {
		if failure(e.Kind) && e.Kind != TimerPassed && (closes || e.Kind != Closed) {
			times = append(times, e.At.Boot)
		}
		bound := time.Duration(0)
		switch {
		case e.Kind == CallSeen && harness == Claude:
			bound = HelloAtCall
		case e.Kind == SessionStarted && harness == Claude, e.Kind == ThreadAdmitted && harness == Codex:
			bound = HelloAtStart
		}
		if end := e.At.Boot + int64(bound); bound != 0 && end <= clock {
			times = append(times, end)
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
// order, from the same launch; the exit, which ends what is of the run,
// stays last.
func foldedAs(folded []Event, r Record, arrange func([]Event)) Record {
	launch := Stamp{}
	if r.Tool == ToolNone {
		launch = r.Interval
	}
	again := New(r.Harness, r.Tool != ToolNone, r.Reason, launch)
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

// unconditional says whether an event is a failure whatever the record
// holds: the server's own report, or a call refused before its ticket.
func unconditional(k Kind) bool { return k == CannotStart || k == NotObserved }

// equal compares what a record shows and keeps: every exported field.
func equal(a, b Record) bool {
	shells := a.Shell == nil && b.Shell == nil ||
		a.Shell != nil && b.Shell != nil && a.Shell.OK == b.Shell.OK && a.Shell.Class == b.Shell.Class && same(a.Shell.At, b.Shell.At)
	return shells && a.Harness == b.Harness && a.Tool == b.Tool && a.Class == b.Class && same(a.ClassAt, b.ClassAt) &&
		a.Reason == b.Reason && same(a.Interval, b.Interval) && same(a.Worked, b.Worked) && same(a.Reconnected, b.Reconnected) &&
		slices.Equal(a.Live, b.Live) && a.Generation == b.Generation && a.Timer == b.Timer &&
		same(a.Block, b.Block) && a.Issued == b.Issued && a.Frozen == b.Frozen && a.Conversation == b.Conversation
}

func same(a, b Stamp) bool { return a.Boot == b.Boot && a.Wall.Equal(b.Wall) }

// equal is written out field by field for speed; a field added to the
// record without it would be compared by nothing.
func TestEqualComparesEveryField(t *testing.T) {
	if n := reflect.TypeFor[Record]().NumField(); n != 17 {
		t.Fatalf("the record has %d fields; equal compares 16 and the history", n)
	}
}
