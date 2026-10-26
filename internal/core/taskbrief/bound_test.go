package taskbrief

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// It broke (review of the briefs step): a tick stored a long item's text
// whole, the entry passed what a reader takes, and the brief no longer read
// for anyone — edit and close included. A text main may write must stay
// tickable; escaping makes ~12 KB of "<" past the bound.
func TestATickOfALongItemLeavesTheBriefReadable(t *testing.T) {
	for _, text := range []string{"- [ ] " + strings.Repeat("x", MaxText-6), "- [ ] " + strings.Repeat("<", 12000)} {
		dir := room(t)
		brief := made(t, dir, text)
		item, err := Tick(dir, brief.ID, worker, 1, 1, true)
		if err != nil || !item.Done {
			t.Fatalf("tick of a %d-byte item: %+v, %v", len(text), item, err)
		}
		read := loaded(t, dir, brief.ID)
		if read.Done() != 1 {
			t.Fatalf("the tick does not count: %+v", read.Items())
		}
		echoed := read.Log[len(read.Log)-1].ItemText
		if len([]rune(echoed)) != maxItemEcho || !strings.HasSuffix(echoed, "…") {
			t.Fatalf("the tick echoes %d code points: %.40q", len([]rune(echoed)), echoed)
		}
		if _, err := Close(dir, brief.ID); err != nil {
			t.Fatalf("close after the tick: %v", err)
		}
	}
}

// A write refuses, writing nothing, a record its readers would refuse: here
// brief.json grown by bindings past the bound.
func TestARecordPastTheBoundIsRefusedUnwritten(t *testing.T) {
	dir := room(t)
	brief := made(t, dir, "- [ ] one\n")
	var err error
	bound := 1
	for ; bound < 2000; bound++ {
		if err = Bind(dir, brief.ID, Author{Name: fmt.Sprintf("run-%d", bound), Epoch: "300.3.boot"}); err != nil {
			break
		}
	}
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("after %d bindings: %v", bound, err)
	}
	read := loaded(t, dir, brief.ID)
	if len(read.Bound) != bound {
		t.Fatalf("%d bindings kept, %d accepted", len(read.Bound), bound)
	}
}
