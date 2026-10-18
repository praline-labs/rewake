package terminal

import (
	"image/color"
	"io"
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// Canvas is the person's terminal as the host mode draws it
// (docs/v2/host-mode.md): a composed screen on the alternate screen, diffed
// against what is shown by ultraviolet's renderer, with the shown program's
// input modes mirrored so its keys arrive in the form it asked for.
type Canvas struct {
	r    *uv.TerminalRenderer
	buf  *uv.RenderBuffer
	size Size
	// modes are the ones set on the person's terminal now.
	modes Modes
	// shown says the cursor is visible on the person's terminal now.
	shown bool
}

// Frame is one composed screen: the shown program's rows in the region at
// the top left, and the bar on the last row.
type Frame struct {
	Region Size
	Rows   [][]Run
	// Cursor is in the region's coordinates; one outside it is not shown.
	Cursor Cursor
	// Modes are the shown program's, mirrored on the person's terminal.
	Modes Modes
	// Bar is drawn on the canvas's last row when it lies below the region.
	Bar []Run
}

// NewCanvas draws on out, a terminal of size; env gives the terminal's type
// and the color profile it is drawn for.
func NewCanvas(out io.Writer, env []string, size Size) *Canvas {
	c := &Canvas{r: uv.NewTerminalRenderer(out, env)}
	c.Resize(size)
	return c
}

// Enter takes the alternate screen, clears it and hides the cursor.
func (c *Canvas) Enter() error {
	c.r.EnterAltScreen()
	_, _ = c.r.WriteString(ansi.HideCursor)
	c.shown = false
	return c.r.Flush()
}

// Leave gives the person's terminal back as it was: every mirrored mode
// reset, whatever was thought set, the cursor shown, the main screen back.
func (c *Canvas) Leave() error {
	_, _ = c.r.WriteString(resetModes + ansi.ResetStyle + ansi.ShowCursor)
	c.modes, c.shown = Modes{}, true
	c.r.ExitAltScreen()
	return c.r.Flush()
}

// Resize takes the person's new size; the next Draw repaints everything.
func (c *Canvas) Resize(size Size) {
	c.size = size
	c.buf = uv.NewRenderBuffer(size.Cols, size.Rows)
	c.r.Resize(size.Cols, size.Rows)
	c.r.Erase()
}

// Redraw has the next Draw repaint everything: a tab switch shows another
// program, and nothing of the last is worth diffing against.
func (c *Canvas) Redraw() { c.r.Erase() }

// Draw shows f: the cells that changed, the modes the shown program set, and
// its cursor.
func (c *Canvas) Draw(f Frame) error {
	for y := range min(f.Region.Rows, c.size.Rows) {
		var runs []Run
		if y < len(f.Rows) {
			runs = f.Rows[y]
		}
		c.line(y, min(f.Region.Cols, c.size.Cols), runs)
	}
	if bar := c.size.Rows - 1; bar >= f.Region.Rows {
		c.line(bar, c.size.Cols, f.Bar)
	}
	if c.shown && c.buf.TouchedLines() > 0 {
		_, _ = c.r.WriteString(ansi.HideCursor)
		c.shown = false
	}
	c.r.Render(c.buf)
	_, _ = c.r.WriteString(modeChanges(c.modes, f.Modes))
	c.modes = f.Modes
	at := f.Cursor
	if at.Visible && at.Row >= 0 && at.Row < f.Region.Rows && at.Col >= 0 && at.Col < f.Region.Cols {
		c.r.MoveTo(at.Col, at.Row)
		if !c.shown {
			_, _ = c.r.WriteString(ansi.ShowCursor)
			c.shown = true
		}
	} else if c.shown {
		_, _ = c.r.WriteString(ansi.HideCursor)
		c.shown = false
	}
	return c.r.Flush()
}

// line sets row y from runs over width columns, blanks past the runs' end.
func (c *Canvas) line(y, width int, runs []Run) {
	x := 0
	for _, run := range runs {
		for _, cell := range cellsOf(run) {
			if x+cell.Width > width {
				break
			}
			c.buf.SetCell(x, y, &cell)
			x += cell.Width
		}
	}
	for ; x < width; x++ {
		c.buf.SetCell(x, y, &uv.EmptyCell)
	}
}

// cellsOf turns a run back into cells. The emulator gives a wide glyph a run
// of its own counted as its width (vt.go), so a run of one cluster over more
// columns is one wide cell; any other run is a cell per cluster, padded with
// blanks to its width.
func cellsOf(run Run) []uv.Cell {
	style := styleOf(run)
	cells := make([]uv.Cell, 0, run.Cols)
	rest := run.Text
	for rest != "" && len(cells) < run.Cols {
		cluster, _ := ansi.FirstGraphemeCluster(rest, ansi.GraphemeWidth)
		if cluster == "" {
			break
		}
		rest = rest[len(cluster):]
		cells = append(cells, uv.Cell{Content: cluster, Width: 1, Style: style})
	}
	if len(cells) == 1 && rest == "" && run.Cols > 1 {
		cells[0].Width = run.Cols
		return cells
	}
	for len(cells) < run.Cols {
		cells = append(cells, uv.Cell{Content: " ", Width: 1, Style: style})
	}
	return cells
}

func styleOf(run Run) uv.Style {
	s := uv.Style{Fg: colorFor(run.Fg), Bg: colorFor(run.Bg)}
	for _, pair := range []struct {
		mine Attrs
		lib  uint8
	}{{Bold, uv.AttrBold}, {Dim, uv.AttrFaint}, {Italic, uv.AttrItalic}, {Inverse, uv.AttrReverse}, {Strikethrough, uv.AttrStrikethrough}} {
		if run.Attrs&pair.mine != 0 {
			s.Attrs |= pair.lib
		}
	}
	if run.Attrs&Underline != 0 {
		s.Underline = uv.UnderlineSingle
	}
	return s
}

// colorFor keeps an index an index, so the person's own palette paints it
// (the owner's wish, docs/v2/host-mode.md): 0–15 as a basic color, which
// the renderer passes through on every profile, 16–255 as an indexed one,
// passed through on every profile but the 16-color one.
func colorFor(c Color) color.Color {
	switch c.Kind {
	case Indexed:
		if c.Index < 16 {
			return ansi.BasicColor(c.Index)
		}
		return ansi.IndexedColor(c.Index)
	case RGB:
		return color.RGBA{R: c.R, G: c.G, B: c.B, A: 0xff}
	}
	return nil
}

// resetModes resets every mode a canvas may mirror.
var resetModes = modeChanges(Modes{
	CursorKeys: true, Keypad: true, BracketedPaste: true, Mouse: MouseAny, MouseSGR: true, Focus: true,
}, Modes{}) + ansi.ResetModeMouseX10 + ansi.ResetModeMouseNormal + ansi.ResetModeMouseButtonEvent

// modeChanges are the sequences that take the person's terminal from the
// input modes was to now. The screen a program shows is not among them: the
// canvas is always on the alternate screen.
func modeChanges(was, now Modes) string {
	var b strings.Builder
	flag := func(old, cur bool, set, reset string) {
		switch {
		case cur && !old:
			b.WriteString(set)
		case old && !cur:
			b.WriteString(reset)
		}
	}
	flag(was.CursorKeys, now.CursorKeys, ansi.SetModeCursorKeys, ansi.ResetModeCursorKeys)
	flag(was.Keypad, now.Keypad, ansi.KeypadApplicationMode, ansi.KeypadNumericMode)
	flag(was.BracketedPaste, now.BracketedPaste, ansi.SetModeBracketedPaste, ansi.ResetModeBracketedPaste)
	flag(was.Focus, now.Focus, ansi.SetModeFocusEvent, ansi.ResetModeFocusEvent)
	if was.Mouse != now.Mouse {
		if reset := mouseModes[was.Mouse][1]; reset != "" {
			b.WriteString(reset)
		}
		b.WriteString(mouseModes[now.Mouse][0])
	}
	flag(was.MouseSGR, now.MouseSGR, ansi.SetModeMouseExtSgr, ansi.ResetModeMouseExtSgr)
	return b.String()
}

// mouseModes are the set and reset of each tracking mode.
var mouseModes = map[MouseMode][2]string{
	MouseOff:    {"", ""},
	MouseX10:    {ansi.SetModeMouseX10, ansi.ResetModeMouseX10},
	MouseNormal: {ansi.SetModeMouseNormal, ansi.ResetModeMouseNormal},
	MouseButton: {ansi.SetModeMouseButtonEvent, ansi.ResetModeMouseButtonEvent},
	MouseAny:    {ansi.SetModeMouseAnyEvent, ansi.ResetModeMouseAnyEvent},
}
