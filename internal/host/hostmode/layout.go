package hostmode

import (
	"strconv"
	"unicode/utf8"

	"github.com/praline-labs/rewake/internal/host/terminal"
)

// splitFrom is the narrowest terminal that shows main and a worker side by
// side: below it each would be too narrow for a program, and the bar switches
// the whole window between them.
const splitFrom = 120

// layout is where things go on the person's terminal of one size: main's
// region at the left edge, the workers' right of it behind a divider column
// when split, and the bar's row below both. Unsplit, both regions are the
// whole terminal above the bar and the focused program takes it.
type layout struct {
	size terminal.Size
	// bar is the bar's row from 0, -1 on a terminal of one row, which shows
	// the region alone.
	bar int
	// main and work are the sizes main and every worker are kept at; work
	// lies from the column workAt, from 0.
	main, work terminal.Size
	workAt     int
	split      bool
}

// layoutFor lays out a terminal of size, split when asked and the terminal is
// at least splitFrom wide: the workers' half is the narrower one when the
// width is odd, the divider between.
func layoutFor(size terminal.Size, split bool) layout {
	region := size
	l := layout{size: size, bar: -1}
	if size.Rows >= 2 {
		region.Rows, l.bar = size.Rows-1, size.Rows-1
	}
	l.main, l.work = region, region
	if split && size.Cols >= splitFrom {
		l.split = true
		l.work.Cols = (size.Cols - 1) / 2
		l.main.Cols = size.Cols - 1 - l.work.Cols
		l.workAt = l.main.Cols + 1
	}
	return l
}

// inRegion says a mouse report at the 1-based row belongs to a program
// rather than to the bar.
func (l layout) inRegion(row int) bool { return row >= 1 && row <= l.main.Rows }

// label is one tab as the bar shows it.
type label struct {
	name string
	// focused takes the keys; shown is on the screen without them, the other
	// half of a split.
	focused, shown bool
	ended          bool
	// busy says the worker is in a turn.
	busy bool
	// changed says the screen changed since the tab was last shown.
	changed bool
}

// The hint is said at the bar's right end when there is room for it, with
// the focus and zoom keys where the terminal is wide enough for a split.
const (
	hint      = " Ctrl+\\ n p 0-9 "
	splitHint = " Ctrl+\\ n p 0-9 ← → z "
)

// barRuns lays the tabs out left to right, main first numbered 0, each as
// " <n> <name> " with a mark when the worker is busy and when it changed
// unseen; the focused one inverted, the other shown one underlined, an ended
// one dim. What does not fit the width is cut; the hint takes the right end
// only when every tab fits beside it.
func barRuns(labels []label, width int, hint string) []terminal.Run {
	var runs []terminal.Run
	used := 0
	for i, l := range labels {
		text := " " + strconv.Itoa(i) + " " + l.name
		if l.busy {
			text += "…"
		}
		if l.changed {
			text += "*"
		}
		text += " "
		var attrs terminal.Attrs
		switch {
		case l.focused:
			attrs |= terminal.Inverse | terminal.Bold
		case l.shown:
			attrs |= terminal.Underline | terminal.Bold
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
	if gap := width - used - utf8.RuneCountInString(hint); gap >= 0 && len(runs) == len(labels) {
		runs = append(runs,
			terminal.Run{Text: spaces(gap), Cols: gap},
			terminal.Run{Text: hint, Cols: utf8.RuneCountInString(hint), Attrs: terminal.Dim})
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
