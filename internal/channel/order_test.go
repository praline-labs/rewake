package channel

import (
	"testing"
	"time"
)

// Rule 5 of docs/mail-bridge-channel.md: events are ordered by when they
// happened, not by when they were folded. Every few events at distinct
// times — hellos and closes of two connections, the timer from the launch,
// every failure, the ticket, the denial and the shell's observations —
// folded in each of their arrival orders, leave the record that folding
// them by event time leaves.

func TestEveryArrivalOrderFoldsAsEventTime(t *testing.T) {
	t.Parallel()
	kinds := []Event{
		{Kind: Hello, Generation: 1},
		{Kind: Hello, Generation: 2},
		{Kind: Closed, Generation: 1, Alive: true},
		{Kind: Closed, Generation: 2, Alive: true},
		{Kind: HelloRefused, Descendant: true},
		{Kind: HelloRefused},
		{Kind: Validated},
		{Kind: Denied},
		{Kind: ShellObserved, OK: true},
		{Kind: ShellObserved, Class: ShellReadOnly},
		{Kind: TimerPassed},
	}
	length := 4
	if testing.Short() {
		length = 3
	}
	orders := permutations(length)
	cases := 0
	for picks := range combinations(len(kinds), length) {
		events, ok := atTimes(kinds, picks)
		if !ok {
			continue
		}
		fold := func(order []int) Record {
			r := New(true, "", stampAt(0))
			for _, index := range order {
				r.Fold(events[index])
			}
			return r
		}
		want := fold(orders[0])
		for _, order := range orders[1:] {
			if got := fold(order); !equal(got, want) {
				t.Fatalf("%v in order %v:\n got  %+v\n want %+v", events, order, got, want)
			}
			cases++
		}
	}
	t.Logf("%d arrival orders", cases)
}

// atTimes places the picked events eight seconds apart, so the timer's end
// from the launch falls between the first two, and says whether they make
// a run: a connection's hello once, before its close, which comes once.
func atTimes(kinds []Event, picks []int) ([]Event, bool) {
	events := make([]Event, len(picks))
	hellos, closes := map[uint64]int{}, map[uint64]int{}
	for i, k := range picks {
		e := kinds[k]
		e.At = stampAt(time.Duration(i+1) * 8 * time.Second)
		if e.Kind == Validated {
			e.Issued = e.At.Boot - int64(time.Second/2)
		}
		switch e.Kind {
		case Hello:
			if _, seen := hellos[e.Generation]; seen {
				return nil, false
			}
			hellos[e.Generation] = i
		case Closed:
			if _, seen := closes[e.Generation]; seen {
				return nil, false
			}
			closes[e.Generation] = i
		}
		events[i] = e
	}
	for g, at := range closes {
		if hello, seen := hellos[g]; seen && hello > at {
			return nil, false
		}
	}
	return events, true
}

// combinations yields every sequence of n picks from k, repeats allowed.
func combinations(k, n int) func(yield func([]int) bool) {
	return func(yield func([]int) bool) {
		picks := make([]int, n)
		for {
			if !yield(picks) {
				return
			}
			i := n - 1
			for ; i >= 0 && picks[i] == k-1; i-- {
				picks[i] = 0
			}
			if i < 0 {
				return
			}
			picks[i]++
		}
	}
}

// permutations are every order of n indexes, the identity first.
func permutations(n int) [][]int {
	if n == 0 {
		return [][]int{{}}
	}
	var all [][]int
	for _, rest := range permutations(n - 1) {
		for at := len(rest); at >= 0; at-- {
			order := append(append(append([]int{}, rest[:at]...), n-1), rest[at:]...)
			all = append(all, order)
		}
	}
	return all
}
