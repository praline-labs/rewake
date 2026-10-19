package hostmode

import "github.com/praline-labs/rewake/internal/host/terminal"

// The commands after the prefix that are not a key's own letter or digit:
// the left and right arrows.
const (
	cmdLeft byte = 0x80 + iota
	cmdRight
)

var (
	focusIn  = []byte("\x1b[I")
	focusOut = []byte("\x1b[O")
)

// pane is a program on the screen: its tab, the column it starts at from 0,
// and its size.
type pane struct {
	tab  *tab
	col  int
	size terminal.Size
}

// panes are the programs on the screen, left to right: main and the chosen
// worker when split, the focused program alone otherwise.
func (h *Host) panes() []pane {
	if h.lay.split {
		return []pane{{tab: h.tabs[0], size: h.lay.main}, {tab: h.chosen, col: h.lay.workAt, size: h.lay.work}}
	}
	return []pane{{tab: h.focus, size: h.lay.main}}
}

// paneAt is the pane covering the column col, from 1; the divider is no
// pane's.
func (h *Host) paneAt(col int) (pane, bool) {
	for _, p := range h.panes() {
		if col > p.col && col <= p.col+p.size.Cols {
			return p, true
		}
	}
	return pane{}, false
}

func (h *Host) shown(t *tab) bool {
	for _, p := range h.panes() {
		if p.tab == t {
			return true
		}
	}
	return false
}

// command runs a key typed after the prefix. A digit naming no tab, and an
// arrow to a worker before there is one, do nothing.
func (h *Host) command(c byte) {
	at := 0
	for i, t := range h.tabs {
		if t == h.focus {
			at = i
		}
	}
	switch {
	case c == 'n':
		h.show(h.tabs[(at+1)%len(h.tabs)])
	case c == 'p':
		h.show(h.tabs[(at+len(h.tabs)-1)%len(h.tabs)])
	case c == 'm' || c == '0' || c == cmdLeft:
		h.show(h.tabs[0])
	case c >= '1' && c <= '9' && int(c-'0') < len(h.tabs):
		h.show(h.tabs[c-'0'])
	case c == cmdRight && h.chosen != nil:
		h.show(h.chosen)
	case c == 'z':
		h.zoom = !h.zoom
		h.relayout(h.lay.size)
	}
}

// show focuses t, a worker becoming the one the right half shows.
func (h *Host) show(t *tab) {
	if t != h.tabs[0] && t != h.chosen {
		h.chosen = t
		h.canvas.Redraw()
		h.dirty = true
	}
	h.focusOn(t)
}

// focusOn gives t the keys, telling each program that asked for focus
// reports (1004) that it lost or gained it.
func (h *Host) focusOn(t *tab) {
	if t == h.focus {
		return
	}
	if h.focus.modes().Focus {
		h.focus.send(focusOut)
	}
	if t.modes().Focus {
		t.send(focusIn)
	}
	if !h.lay.split {
		h.canvas.Redraw()
	}
	h.focus = t
	h.dirty = true
}

// relayout lays the terminal of size out again, split unless zoomed or there
// is no worker, and sizes every program to its region.
func (h *Host) relayout(size terminal.Size) {
	lay := layoutFor(size, !h.zoom && h.chosen != nil)
	if lay != h.lay {
		h.canvas.Redraw()
	}
	h.lay = lay
	for i, t := range h.tabs {
		if i == 0 {
			t.size(lay.main)
		} else {
			t.size(lay.work)
		}
	}
	h.dirty = true
}

// mouse routes a report: a click on the bar shows the tab under it; an event
// in a pane goes to its program, at the pane's own cell and as it asked to
// hear it, a left click focusing it; while a button is down every event goes
// to the program it went down on, held inside its pane. Anything else is
// dropped.
func (h *Host) mouse(m mouseEvent) {
	if m.row-1 == h.lay.bar {
		if m.leftClick() {
			if i := tabAt(h.barRuns(), len(h.tabs), m.col); i >= 0 {
				h.show(h.tabs[i])
			}
		}
		if m.release {
			h.grab = nil
		}
		return
	}
	if !h.lay.inRegion(m.row) {
		return
	}
	p, ok := h.paneAt(m.col)
	if h.grab != nil {
		ok = false
		for _, q := range h.panes() {
			if q.tab == h.grab {
				p, ok = q, true
			}
		}
		if !ok {
			// The program it went down on is off the screen now.
			h.grab = nil
		}
	}
	if !ok {
		return
	}
	switch {
	case m.release:
		h.grab = nil
	case m.press() && m.code&mouseWheel == 0:
		h.grab = p.tab
	}
	if m.leftClick() {
		h.focusOn(p.tab)
	}
	col := min(max(m.col-p.col, 1), p.size.Cols)
	if b := m.encode(p.tab.modes(), col, m.row); b != nil {
		p.tab.send(b)
	}
}

func (h *Host) label(t *tab) label {
	shown := h.shown(t)
	return label{
		name: t.name, focused: t == h.focus, shown: shown, ended: t.pic.Ended,
		busy: t.info.Activity == "working", changed: !shown && t.pic.Seq != t.seen,
	}
}

func (h *Host) barRuns() []terminal.Run {
	labels := make([]label, len(h.tabs))
	for i, t := range h.tabs {
		labels[i] = h.label(t)
	}
	help := hint
	if h.lay.size.Cols >= splitFrom {
		help = splitHint
	}
	return barRuns(labels, h.lay.size.Cols, help)
}

// draw composes the screen: each pane's program, a dim divider between, the
// bar, and the focused program's cursor at its place in its pane.
func (h *Host) draw() {
	h.dirty = false
	var panes []terminal.Pane
	var cursor terminal.Cursor
	for _, p := range h.panes() {
		if p.col > 0 {
			panes = append(panes, divider(p.col-1, p.size.Rows))
		}
		pic := p.tab.pic
		p.tab.seen = pic.Seq
		panes = append(panes, terminal.Pane{Col: p.col, Size: p.size, Rows: pic.Rows})
		at := pic.Cursor
		if p.tab == h.focus && !pic.Ended && at.Visible && at.Row < p.size.Rows && at.Col < p.size.Cols {
			cursor = terminal.Cursor{Row: at.Row, Col: p.col + at.Col, Visible: true}
		}
	}
	_ = h.canvas.Draw(terminal.Frame{
		Panes: panes, Cursor: cursor, Modes: h.focus.modes(), Bar: h.barRuns(), Title: h.focus.pic.Title,
	})
}

// divider is the column between the halves, as a pane of its own.
func divider(col, rows int) terminal.Pane {
	line := make([][]terminal.Run, rows)
	for i := range line {
		line[i] = []terminal.Run{{Text: "│", Cols: 1, Attrs: terminal.Dim}}
	}
	return terminal.Pane{Col: col, Size: terminal.Size{Cols: 1, Rows: rows}, Rows: line}
}
