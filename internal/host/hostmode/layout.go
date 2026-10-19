package hostmode

import (
	"strconv"
	"unicode/utf8"

	"github.com/praline-labs/rewake/internal/host/terminal"
)

const (
	// sideWidth is the sidebar's width, its border included.
	sideWidth = 28
	// sideFrom is the narrowest terminal that shows the sidebar: below it
	// the region would be too narrow for a program, and the bar alone
	// navigates.
	sideFrom = 120
)

// layout is where things go on the person's terminal of one size: the
// region every program is sized to at the top left, the sidebar right of
// it, and the bar's row below both.
type layout struct {
	size   terminal.Size
	region terminal.Size
	// bar is the bar's row from 0, -1 on a terminal of one row, which shows
	// the region alone.
	bar int
	// side is the sidebar's width, 0 when it is not shown.
	side int
}

// layoutFor lays out a terminal of size, the sidebar shown unless hidden by
// the person or the terminal is narrower than sideFrom.
func layoutFor(size terminal.Size, hidden bool) layout {
	l := layout{size: size, region: size, bar: -1}
	if size.Rows >= 2 {
		l.region.Rows, l.bar = size.Rows-1, size.Rows-1
	}
	if !hidden && size.Cols >= sideFrom {
		l.side = sideWidth
		l.region.Cols -= sideWidth
	}
	return l
}

// inRegion says a mouse report at the 1-based row belongs to the shown
// program rather than to the bar.
func (l layout) inRegion(row int) bool { return row >= 1 && row <= l.region.Rows }

// label is one tab as the bar shows it.
type label struct {
	name    string
	current bool
	ended   bool
	// changed says the screen changed since the tab was last shown.
	changed bool
}

// The hint is said at the bar's right end when there is room for it, with
// the sidebar's key where the terminal is wide enough for one.
const (
	hint     = " Ctrl+\\ n p 0-9 "
	sideHint = " Ctrl+\\ n p 0-9 s "
)

// barRuns lays the tabs out left to right, main first numbered 0, each as
// " <n> <name> " with a mark when it changed unseen; the current one inverted,
// an ended one dim. What does not fit the width is cut; the hint takes the
// right end only when every tab fits beside it.
func barRuns(labels []label, width int, hint string) []terminal.Run {
	var runs []terminal.Run
	used := 0
	for i, l := range labels {
		text := " " + strconv.Itoa(i) + " " + l.name
		if l.changed {
			text += "*"
		}
		text += " "
		var attrs terminal.Attrs
		if l.current {
			attrs |= terminal.Inverse | terminal.Bold
		}
		if l.ended {
			attrs |= terminal.Dim
		}
		text = cut(text, width-used)
		if text == "" {
			break
		}
		cols := utf8.RuneCountInString(text)
		runs = append(runs, terminal.Run{Text: text, Cols: cols, Attrs: attrs})
		used += cols
	}
	if gap := width - used - len(hint); gap >= 0 && len(runs) == len(labels) {
		runs = append(runs,
			terminal.Run{Text: spaces(gap), Cols: gap},
			terminal.Run{Text: hint, Cols: len(hint), Attrs: terminal.Dim})
	}
	return runs
}

// tabAt is the tab whose label covers the column col, from 1, of a bar laid
// out by barRuns for tabs tabs, or -1: its first runs are the tabs' labels in
// order, as many as fit.
func tabAt(runs []terminal.Run, tabs, col int) int {
	end := 0
	for i := range min(tabs, len(runs)) {
		end += runs[i].Cols
		if col <= end {
			return i
		}
	}
	return -1
}

// cut keeps at most cols runes of s. A tab's name is a session's or a
// space's, which rewake keeps to narrow characters, so a rune is a column.
func cut(s string, cols int) string {
	if cols <= 0 {
		return ""
	}
	n := 0
	for i := range s {
		if n == cols {
			return s[:i]
		}
		n++
	}
	return s
}

func spaces(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}
