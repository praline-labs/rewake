package hostmode

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/host/spacehost"
	"github.com/praline-labs/rewake/internal/host/terminal"
)

// The sidebar takes its width from the region from sideFrom columns on,
// unless hidden; below that the bar alone navigates.
func TestTheSidebarTakesItsWidthOnAWideTerminal(t *testing.T) {
	wide := terminal.Size{Cols: 130, Rows: 30}
	if l := layoutFor(wide, false); l.side != sideWidth || l.region != (terminal.Size{Cols: 130 - sideWidth, Rows: 29}) {
		t.Fatalf("wide: %+v", l)
	}
	if l := layoutFor(wide, true); l.side != 0 || l.region.Cols != 130 {
		t.Fatalf("hidden: %+v", l)
	}
	if l := layoutFor(terminal.Size{Cols: sideFrom - 1, Rows: 30}, false); l.side != 0 || l.region.Cols != sideFrom-1 {
		t.Fatalf("narrow: %+v", l)
	}
}

// Each row covers the sidebar's width and names the tab it belongs to: main
// in one row, a worker in three, a blank after each.
func TestTheSidebarListsTheTabsAndOwnsTheirRows(t *testing.T) {
	entries := []entry{
		{label: label{name: "main"}},
		{label: label{name: "w1-claude", current: true}, worker: true, info: spacehost.Worker{Harness: "claude", Model: "haiku", Effort: "high", Activity: "working", Percent: 23}},
		{label: label{name: "w2-pi", ended: true}, worker: true, info: spacehost.Worker{Harness: "pi", State: "open", Percent: -1}},
	}
	rows, owner := sideRows(entries, sideWidth, 12)
	if len(rows) != 12 || len(owner) != 12 {
		t.Fatalf("%d rows, %d owners", len(rows), len(owner))
	}
	var text []string
	for _, row := range rows {
		n, s := cols(row)
		if n != sideWidth {
			t.Fatalf("row %q covers %d columns", s, n)
		}
		text = append(text, strings.TrimRight(s, " "))
	}
	want := []string{"│ 0 main", "│", "│ 1 w1-claude", "│   claude · haiku · high", "│   working · 23%", "│", "│ 2 w2-pi", "│   pi", "│   ended", "│", "│", "│"}
	if strings.Join(text, "\n") != strings.Join(want, "\n") {
		t.Fatalf("sidebar:\n%s", strings.Join(text, "\n"))
	}
	wantOwner := []int{0, -1, 1, 1, 1, -1, 2, 2, 2, -1, -1, -1}
	for i := range wantOwner {
		if owner[i] != wantOwner[i] {
			t.Fatalf("owners %v, want %v", owner, wantOwner)
		}
	}
	if rows[2][1].Attrs&terminal.Inverse == 0 || rows[6][1].Attrs&terminal.Dim == 0 {
		t.Fatalf("attributes: current %+v, ended %+v", rows[2], rows[6])
	}
	// A worker that does not fit whole is left out.
	if _, owner := sideRows(entries, sideWidth, 4); owner[2] != -1 || owner[3] != -1 {
		t.Fatalf("a cut worker owns rows: %v", owner)
	}
}

func TestAClickOnTheSidebarShowsItsTabAndNeverReachesTheProgram(t *testing.T) {
	h, sent := host(terminal.Modes{Mouse: terminal.MouseAny, MouseSGR: true})
	h.lay = layoutFor(terminal.Size{Cols: 130, Rows: 10}, false)
	h.tabs[1].space = "s1"
	// The sidebar starts at column 103; w1's name is its third row.
	h.dispatch(h.in.feed([]byte("\x1b[<0;110;3M\x1b[<0;110;3m"), h.in.heldSince))
	drain(h)
	if h.cur != h.tabs[1] || len(*sent) != 0 {
		t.Fatalf("shown %s, sent %q", h.cur.name, *sent)
	}
}
