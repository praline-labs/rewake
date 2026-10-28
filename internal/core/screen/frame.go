// Package screen is a picture of a session's window as cells: the model the
// shell and a space host answer rewake screen with, its text, and its PNG
// (docs/screen.md). It knows no harness and nothing of how a window is drawn.
package screen

import (
	"fmt"
	"strings"
)

// The bounds of a frame, so a picture stays one an agent can open: at most
// 400 by 200 cells, a PNG of at most 3200 by 3200 pixels.
const (
	MaxCols = 400
	MaxRows = 200
)

// The sources of a frame: the shell's window as the person has it, or the
// worker's pane alone from its space host when no shell answers.
const (
	SourceShell = "shell"
	SourceHost  = "host"
)

// The attributes a run carries.
const (
	Bold = 1 << iota
	Dim
	Italic
	Underline
	Inverse
	Strike
)

// Run is a stretch of a line in one style. W counts columns, a wide glyph
// as two; the runs of a line add up to at most the frame's width, the rest
// being the default background. Fg and Bg are "#rrggbb", empty for the
// frame's default.
type Run struct {
	T  string `json:"t"`
	W  int    `json:"w"`
	Fg string `json:"fg,omitempty"`
	Bg string `json:"bg,omitempty"`
	A  int    `json:"a,omitempty"`
}

// Cursor is a cell, from 0.
type Cursor struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Frame is a window as cells.
type Frame struct {
	// Source says who drew it.
	Source string `json:"source"`
	// Pane names the session whose window it is.
	Pane string `json:"pane,omitempty"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
	// Fg and Bg are the default colors, empty where nobody said.
	Fg string `json:"fg,omitempty"`
	Bg string `json:"bg,omitempty"`
	// Palette is the person's terminal's sixteen colors as it reported them,
	// "#rrggbb", an entry empty where it did not. Cells carry their colors
	// resolved; the list says which of the sixteen were the terminal's own.
	Palette []string `json:"palette,omitempty"`
	Cursor  *Cursor  `json:"cursor,omitempty"`
	Lines   [][]Run  `json:"lines"`
}

// StandIn names the colors the picture draws in a stand-in of its own
// because the terminal did not report them: the default foreground and
// background and the palette (every entry must be there to count as
// reported).
func (f *Frame) StandIn() []string {
	var out []string
	if f.Fg == "" {
		out = append(out, "foreground")
	}
	if f.Bg == "" {
		out = append(out, "background")
	}
	reported := len(f.Palette) == 16
	for _, c := range f.Palette {
		reported = reported && c != ""
	}
	if !reported {
		out = append(out, "palette")
	}
	return out
}

// Check refuses a frame outside the bounds or not made of its lines.
func (f *Frame) Check() error {
	if f.Cols < 1 || f.Rows < 1 || f.Cols > MaxCols || f.Rows > MaxRows {
		return fmt.Errorf("a screen of %dx%d cells is outside 1..%dx1..%d", f.Cols, f.Rows, MaxCols, MaxRows)
	}
	if len(f.Lines) != f.Rows {
		return fmt.Errorf("a screen of %d rows has %d lines", f.Rows, len(f.Lines))
	}
	for y, line := range f.Lines {
		width := 0
		for _, run := range line {
			if run.W < 0 {
				return fmt.Errorf("line %d has a run of %d columns", y, run.W)
			}
			width += run.W
		}
		if width > f.Cols {
			return fmt.Errorf("line %d is %d columns in a screen of %d", y, width, f.Cols)
		}
	}
	if c := f.Cursor; c != nil && (c.X < 0 || c.Y < 0 || c.X >= f.Cols || c.Y >= f.Rows) {
		return fmt.Errorf("the cursor %d,%d is outside the screen", c.X, c.Y)
	}
	return nil
}

// Text is the screen as lines of plain text, each less its trailing blanks.
func (f *Frame) Text() []string {
	out := make([]string, len(f.Lines))
	for y, line := range f.Lines {
		var b strings.Builder
		for _, run := range line {
			b.WriteString(run.T)
		}
		out[y] = strings.TrimRight(b.String(), " ")
	}
	return out
}
