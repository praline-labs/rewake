package hostmode

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/praline-labs/rewake/internal/host/spacehost"
	"github.com/praline-labs/rewake/internal/host/terminal"
)

// entry is one tab as the sidebar lists it: its label, and for a worker what
// its space and snapshot say.
type entry struct {
	label
	// worker is false for main, which the sidebar names alone.
	worker bool
	info   spacehost.Worker
}

// sideRows lays the entries out top down in a sidebar of width by height,
// each covering its width behind a border: a worker as its numbered name,
// then harness, model and effort, then its turn or its space's state and how
// full its context is, then a blank; main as its name and a blank. The
// current entry's name is inverted, an ended one dim, as on the bar. owner
// names the tab each row belongs to, -1 for none; entries that do not fit
// are left out.
func sideRows(entries []entry, width, height int) (rows [][]terminal.Run, owner []int) {
	rows = make([][]terminal.Run, 0, height)
	owner = make([]int, 0, height)
	add := func(tab int, text string, attrs terminal.Attrs) {
		if len(rows) < height {
			rows = append(rows, sideRow(text, width, attrs))
			owner = append(owner, tab)
		}
	}
	for i, e := range entries {
		lines := 2
		if e.worker {
			lines = 4
		}
		if len(rows)+lines-1 > height {
			break
		}
		name := " " + strconv.Itoa(i) + " " + e.name
		if e.changed {
			name += "*"
		}
		var attrs terminal.Attrs
		if e.current {
			attrs |= terminal.Inverse | terminal.Bold
		}
		if e.ended {
			attrs |= terminal.Dim
		}
		add(i, name, attrs)
		if e.worker {
			add(i, "   "+joinSet(e.info.Harness, e.info.Model, e.info.Effort), terminal.Dim)
			add(i, "   "+e.status(), terminal.Dim)
		}
		add(-1, "", 0)
	}
	for len(rows) < height {
		add(-1, "", 0)
	}
	return rows, owner
}

// status is a worker's turn, or its space's state while it has none, and
// how full its context is when known.
func (e entry) status() string {
	state := e.info.Activity
	if state == "" {
		state = e.info.State
	}
	if e.ended {
		state = "ended"
	}
	context := ""
	if e.info.Percent >= 0 {
		context = strconv.Itoa(e.info.Percent) + "%"
	}
	return joinSet(state, context)
}

// sideRow is one row of the sidebar: the border, then text cut or padded to
// the rest of the width, all of it in attrs but the border.
func sideRow(text string, width int, attrs terminal.Attrs) []terminal.Run {
	if width <= 0 {
		return nil
	}
	runs := []terminal.Run{{Text: "│", Cols: 1, Attrs: terminal.Dim}}
	text = cut(text, width-1)
	cols := utf8.RuneCountInString(text)
	if attrs&terminal.Inverse != 0 {
		// The current entry is inverted across the sidebar, as a selection.
		return append(runs, terminal.Run{Text: text + spaces(width-1-cols), Cols: width - 1, Attrs: attrs})
	}
	if cols > 0 {
		runs = append(runs, terminal.Run{Text: text, Cols: cols, Attrs: attrs})
	}
	if rest := width - 1 - cols; rest > 0 {
		runs = append(runs, terminal.Run{Text: spaces(rest), Cols: rest})
	}
	return runs
}

// joinSet joins the parts that are not empty with a middle dot.
func joinSet(parts ...string) string {
	set := parts[:0:0]
	for _, p := range parts {
		if p != "" {
			set = append(set, p)
		}
	}
	return strings.Join(set, " · ")
}
