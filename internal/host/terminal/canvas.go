package terminal

import (
	"image/color"
	"io"
	"strconv"
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// Canvas is the person's terminal as the host mode draws it
// (docs/v2/host-mode.md): a composed screen on the alternate screen, diffed
// against what is shown by ultraviolet's renderer, with the focused program's
// key modes mirrored so its keys arrive in the form it asked for, and the
// mouse and bracketed paste kept on for the host, which routes them.
type Canvas struct {
	r    *uv.TerminalRenderer
	buf  *uv.RenderBuffer
	size Size
	// modes are the ones set on the person's terminal now.
	modes Modes
	// shown says the cursor is visible on the person's terminal now.
	shown bool
	// title is the one set on the person's terminal now.
	title string
}

// Frame is one composed screen: the programs shown side by side above the
// bar, and the bar on the last row.
type Frame struct {
	// Panes are the programs' screens, left to right; a cell above the bar
	// no pane covers is blank.
	Panes []Pane
	// Cursor is in the canvas's coordinates, shown only above the bar.
	Cursor Cursor
	// Modes are the focused program's: its key and focus modes are mirrored
	// on the person's terminal, its mouse mode chooses only whether motion
	// without a button is reported (hostModes).
	Modes Modes
	// Bar is drawn on the canvas's last row when it has more than one.
	Bar []Run
	// Title is the focused program's, set on the person's terminal.
	Title string
}

// Pane is one program's screen from the canvas's top row at the column Col,
// of Size; its rows past Size are cut.
type Pane struct {
	Col  int
	Size Size
	Rows [][]Run
}

// The title stack of xterm's window operations: the person's title is saved
// on entering and given back on leaving, whatever the programs set.
const (
	pushTitle = "\x1b[22;0t"
	popTitle  = "\x1b[23;0t"
)

// NewCanvas draws on out, a terminal of size; env gives the terminal's type
// and the color profile it is drawn for.
func NewCanvas(out io.Writer, env []string, size Size) *Canvas {
	c := &Canvas{r: uv.NewTerminalRenderer(out, env)}
	c.Resize(size)
	return c
}

// Enter takes the alternate screen, clears it, hides the cursor and turns
// the host's mouse and bracketed paste on.
func (c *Canvas) Enter() error {
	c.r.EnterAltScreen()
	c.modes = hostModes(Modes{})
	_, _ = c.r.WriteString(pushTitle + ansi.HideCursor + modeChanges(Modes{}, c.modes))
	c.shown = false
	return c.r.Flush()
}

// Leave gives the person's terminal back as it was: every mirrored mode
// reset, whatever was thought set, the cursor shown, the main screen back.
// The kitty keyboard flags are popped only when the canvas pushed them: a pop
// too many would take what the person's own shell pushed.
func (c *Canvas) Leave() error {
	var kitty string
	if c.modes.KittyKeys != 0 {
		kitty = ansi.PopKittyKeyboard(1)
	}
	_, _ = c.r.WriteString(kitty + resetModes + ansi.ResetStyle + ansi.ShowCursor + popTitle)
	c.modes, c.shown, c.title = Modes{}, true, ""
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

// Draw shows f: the cells that changed, the modes the focused program set,
// and its cursor.
func (c *Canvas) Draw(f Frame) error {
	above := c.size.Rows
	if above > 1 {
		above--
		c.line(0, above, c.size.Cols, f.Bar)
	}
	for y := range above {
		x := 0
		for _, p := range f.Panes {
			// Panes come left to right without overlap; one that does
			// not is left out.
			if p.Col < x || p.Col >= c.size.Cols {
				continue
			}
			c.line(x, y, p.Col-x, nil)
			var runs []Run
			if y < p.Size.Rows && y < len(p.Rows) {
				runs = p.Rows[y]
			}
			width := min(p.Size.Cols, c.size.Cols-p.Col)
			c.line(p.Col, y, width, runs)
			x = p.Col + width
		}
		c.line(x, y, c.size.Cols-x, nil)
	}
	if c.shown && c.buf.TouchedLines() > 0 {
		_, _ = c.r.WriteString(ansi.HideCursor)
		c.shown = false
	}
	c.r.Render(c.buf)
	want := hostModes(f.Modes)
	_, _ = c.r.WriteString(modeChanges(c.modes, want))
	c.modes = want
	if f.Title != c.title {
		_, _ = c.r.WriteString(ansi.SetWindowTitle(f.Title))
		c.title = f.Title
	}
	at := f.Cursor
	if at.Visible && at.Row >= 0 && at.Row < above && at.Col >= 0 && at.Col < c.size.Cols {
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

// Event passes on what a program asked of its terminal: a bell, or an OSC
// string as the program wrote it, ended by BEL.
func (c *Canvas) Event(e Event) error {
	switch {
	case e.Bell:
		_, _ = c.r.WriteString("\a")
	case e.OSC != "":
		_, _ = c.r.WriteString("\x1b]" + e.OSC + "\a")
	}
	return c.r.Flush()
}

// line sets row y from column from on, out of runs over width columns,
// blanks past the runs' end.
func (c *Canvas) line(from, y, width int, runs []Run) {
	x := 0
	for _, run := range runs {
		for _, cell := range cellsOf(run) {
			if x+cell.Width > width {
				break
			}
			c.buf.SetCell(from+x, y, &cell)
			x += cell.Width
		}
	}
	for ; x < width; x++ {
		c.buf.SetCell(from+x, y, &uv.EmptyCell)
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

// hostModes are the modes the person's terminal keeps while the program
// with modes has the focus. The keys' forms and focus reports are the program's;
// the mouse in the SGR form and bracketed paste are the host's, always on,
// so a click on the bar reaches the host and a paste is known as one
// whatever the program asked (docs/v2/host-mode.md#input). The keyboard
// protocols are the program's too, so Shift+Enter reaches it as it asked to
// hear it rather than as a plain Enter. Motion without a
// button is asked for only while the program wants it, so the host is not
// flooded with reports nobody takes.
func hostModes(program Modes) Modes {
	mouse := MouseButton
	if program.Mouse == MouseAny {
		mouse = MouseAny
	}
	return Modes{
		CursorKeys: program.CursorKeys, Keypad: program.Keypad, Focus: program.Focus,
		KittyKeys: program.KittyKeys, ModifyOtherKeys: program.ModifyOtherKeys,
		BracketedPaste: true, Mouse: mouse, MouseSGR: true,
	}
}

// resetModes resets every mode a canvas may set but the kitty keyboard
// flags, which Leave pops only when it pushed them.
var resetModes = modeChanges(Modes{
	CursorKeys: true, Keypad: true, BracketedPaste: true, Mouse: MouseAny, MouseSGR: true, Focus: true,
	ModifyOtherKeys: 2,
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
	// The canvas holds at most one entry on the terminal's kitty stack, its
	// program's flags, pushed over what was there and popped to change them,
	// so the person's own entries stay below untouched.
	if was.KittyKeys != now.KittyKeys {
		if was.KittyKeys != 0 {
			b.WriteString(ansi.PopKittyKeyboard(1))
		}
		if now.KittyKeys != 0 {
			b.WriteString(ansi.PushKittyKeyboard(int(now.KittyKeys)))
		}
	}
	if was.ModifyOtherKeys != now.ModifyOtherKeys {
		if now.ModifyOtherKeys == 0 {
			// The reset gives the terminal's own setting back.
			b.WriteString(ansi.ResetModifyOtherKeys)
		} else {
			b.WriteString("\x1b[>4;" + strconv.Itoa(int(now.ModifyOtherKeys)) + "m")
		}
	}
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
