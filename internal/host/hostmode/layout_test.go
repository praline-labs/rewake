package hostmode

import (
	"testing"

	"github.com/praline-labs/rewake/internal/host/terminal"
)

func TestTheRegionLeavesTheLastRowToTheBar(t *testing.T) {
	l := layoutFor(terminal.Size{Cols: 100, Rows: 30})
	if l.region != (terminal.Size{Cols: 100, Rows: 29}) || l.bar != 29 {
		t.Fatalf("layout %+v", l)
	}
	if !l.inRegion(29) || l.inRegion(30) || l.inRegion(0) {
		t.Fatal("row 29 is the region's last, 30 the bar's")
	}
	one := layoutFor(terminal.Size{Cols: 80, Rows: 1})
	if one.region.Rows != 1 || one.bar != -1 {
		t.Fatalf("a one-row terminal: %+v", one)
	}
}

func cols(runs []terminal.Run) (int, string) {
	n, text := 0, ""
	for _, r := range runs {
		n += r.Cols
		text += r.Text
	}
	return n, text
}

func TestTheBarNumbersTabsAndMarksTheCurrent(t *testing.T) {
	labels := []label{{name: "main"}, {name: "w1-claude", current: true}, {name: "w2-pi", ended: true, changed: true}}
	runs := barRuns(labels, 80)
	n, text := cols(runs)
	if n != 80 {
		t.Fatalf("the bar covers %d columns, want 80", n)
	}
	want := " 0 main  1 w1-claude  2 w2-pi* "
	if text[:len(want)] != want || text[len(text)-len(hint):] != hint {
		t.Fatalf("bar %q", text)
	}
	if runs[1].Attrs&terminal.Inverse == 0 || runs[0].Attrs&terminal.Inverse != 0 || runs[2].Attrs&terminal.Dim == 0 {
		t.Fatalf("attributes %+v", runs)
	}
}

func TestANarrowBarCutsTabsAndDropsTheHint(t *testing.T) {
	labels := []label{{name: "main"}, {name: "w1-claude"}}
	n, text := cols(barRuns(labels, 12))
	if n != 12 || text != " 0 main  1 w" {
		t.Fatalf("bar %q over %d columns", text, n)
	}
	if n, _ := cols(barRuns(labels, 0)); n != 0 {
		t.Fatalf("a bar of no width has %d columns", n)
	}
}

func TestAColumnOfTheBarNamesTheTabUnderIt(t *testing.T) {
	runs := barRuns([]label{{name: "main"}, {name: "w1"}}, 80)
	for col, want := range map[int]int{1: 0, 8: 0, 9: 1, 14: 1, 15: -1, 80: -1} {
		if got := tabAt(runs, 2, col); got != want {
			t.Errorf("column %d: tab %d, want %d", col, got, want)
		}
	}
}
