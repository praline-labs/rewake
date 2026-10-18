package hostmode

import (
	"strconv"
	"unicode/utf8"

	"github.com/praline-labs/rewake/internal/host/terminal"
)

// layout is where things go on the person's terminal of one size: the
// region every program is sized to, and the bar's row below it.
type layout struct {
	size   terminal.Size
	region terminal.Size
	// bar is the bar's row from 0, -1 on a terminal of one row, which shows
	// the region alone.
	bar int
}

func layoutFor(size terminal.Size) layout {
	if size.Rows < 2 {
		return layout{size: size, region: size, bar: -1}
	}
	return layout{size: size, region: terminal.Size{Cols: size.Cols, Rows: size.Rows - 1}, bar: size.Rows - 1}
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

// hint is said at the bar's right end when there is room for it.
const hint = " Ctrl+\\ n p 0-9 "

// barRuns lays the tabs out left to right, main first numbered 0, each as
// " <n> <name> " with a mark when it changed unseen; the current one inverted,
// an ended one dim. What does not fit the width is cut; the hint takes the
// right end only when every tab fits beside it.
func barRuns(labels []label, width int) []terminal.Run {
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
