package proc

import (
	"errors"
	"testing"
)

// A process is shown outside another only from its whole chain: below it,
// a link that cannot be read, a reused pid or a loop is no answer.
func TestOutsideIsShownOnlyFromAWholeChain(t *testing.T) {
	// 1 init; 10 main's wrapper; 20 below it; 40 reparented to init; 50
	// whose parent cannot be read; 60 whose parent is a reused pid.
	r := fixture(t, map[int][2]int{1: {0, 1}, 10: {1, 100}, 20: {10, 200}, 40: {1, 150}, 50: {77, 160}, 61: {1, 900}, 60: {61, 170}})
	if err := r.Outside(40, 10); err != nil {
		t.Fatalf("a process reparented to init: %v", err)
	}
	for _, pid := range []int{10, 20} {
		if err := r.Outside(pid, 10); !errors.Is(err, ErrDescendant) {
			t.Errorf("%d below: %v", pid, err)
		}
	}
	for _, pid := range []int{50, 60, 99} {
		if err := r.Outside(pid, 10); err == nil || errors.Is(err, ErrDescendant) {
			t.Errorf("%d not shown: %v", pid, err)
		}
	}
	loop := fixture(t, map[int][2]int{5: {6, 100}, 6: {5, 100}})
	if err := loop.Outside(5, 10); err == nil {
		t.Fatal("a loop was shown outside")
	}
}

func TestParentReadsBothFieldsOfOneLine(t *testing.T) {
	r := fixture(t, map[int][2]int{30: {20, 300}})
	parent, start, err := r.Parent(30)
	if err != nil || parent != 20 || start != 300 {
		t.Fatalf("Parent = %d, %d, %v", parent, start, err)
	}
}
