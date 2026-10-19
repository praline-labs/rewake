package terminal

import (
	"errors"
	"image/color"
	"io"
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
)

// vtEmulator is the emulator of github.com/charmbracelet/x/vt, pinned at
// v0.1.0: the pseudo-version agent-deck pins loses a combining mark after an
// ASCII base within one write (docs/archive/v2/agent-deck-ideas.md).
type vtEmulator struct {
	emu *vt.Emulator
	// pipe is the write end of the library's reply pipe, closed to end the
	// reply reader. Closing the emulator would do it too, but Close writes a
	// field Read reads without a lock, a race the detector reports.
	pipe io.Closer
	// modes are kept from the mode callbacks, which run inside write, so
	// they change only under the terminal's lock.
	modes Modes
	// kitty are the kitty keyboard flags saved by pushes (keyboard.go).
	kitty []uint8
	osc   oscFilter
	// title is the program's, whole (osc.go).
	title string
	// pen is the style text is written in now, followed from the program's
	// SGR for a late client's preamble (replay.go): the library keeps it
	// unexported.
	pen uv.Style
}

func newVT(size Size) (emulator, error) {
	emu := vt.NewEmulator(size.Cols, size.Rows)
	pipe, ok := emu.InputPipe().(io.Closer)
	if !ok {
		return nil, errors.New("the emulator's reply pipe cannot be closed, so its reader could not be stopped")
	}
	v := &vtEmulator{emu: emu, pipe: pipe}
	// The screen shown is the visible one; the library's default of 10 000
	// lines of scrollback would cost memory and a shift of the whole buffer
	// per scrolled line. One line is the least it takes.
	emu.SetScrollbackSize(1)
	emu.SetCallbacks(vt.Callbacks{
		EnableMode:  func(m ansi.Mode) { v.setMode(m, true) },
		DisableMode: func(m ansi.Mode) { v.setMode(m, false) },
	})
	v.osc.done = v.oscSeen
	// A clipboard request is taken and dropped: there is no clipboard here
	// to set, and none to read back to the program.
	emu.RegisterOscHandler(52, func([]byte) bool { return true })
	// The library answers a mode it does not know as "not recognized", and a
	// program that asks DECRQM 2026 then draws without synchronized output;
	// a client's widget shows each piece of such a frame as it comes. Reset
	// is "recognized, off": the library then follows h and l, and the bytes
	// reach the widget unchanged, which batches the frame itself. This
	// emulator draws at once, which is how a terminal that ignores the
	// batching behaves. A full reset (ESC c) forgets it.
	_, _ = emu.Write([]byte("\x1b[?2026l"))
	// XTVERSION is answered as the widget answers it for main's pane. Claude
	// Code asks DECRQM 2026 and the other mode queries only once it hears a
	// terminal's name, and without them it sends no synchronized output.
	emu.RegisterCsiHandler(ansi.Command('>', 0, 'q'), func(ansi.Params) bool {
		_, _ = io.WriteString(emu.InputPipe(), "\x1bP>|libghostty\x1b\\")
		return true
	})
	v.keyboard()
	v.followPen()
	return v, nil
}

func (v *vtEmulator) setMode(m ansi.Mode, on bool) {
	switch m {
	case ansi.ModeCursorKeys:
		v.modes.CursorKeys = on
	case ansi.ModeNumericKeypad:
		v.modes.Keypad = on
	case ansi.ModeBracketedPaste:
		v.modes.BracketedPaste = on
	case ansi.ModeFocusEvent:
		v.modes.Focus = on
	case ansi.ModeMouseExtSgr:
		v.modes.MouseSGR = on
	case ansi.ModeMouseX10:
		v.mouse(MouseX10, on)
	case ansi.ModeMouseNormal:
		v.mouse(MouseNormal, on)
	case ansi.ModeMouseButtonEvent:
		v.mouse(MouseButton, on)
	case ansi.ModeMouseAnyEvent:
		v.mouse(MouseAny, on)
	}
}

// mouse follows xterm: the last tracking mode set is the one in force, and
// resetting it turns tracking off.
func (v *vtEmulator) mouse(m MouseMode, on bool) {
	switch {
	case on:
		v.modes.Mouse = m
	case v.modes.Mouse == m:
		v.modes.Mouse = MouseOff
	}
}

func (v *vtEmulator) write(p []byte) { _, _ = v.emu.Write(v.osc.filter(p)) }

func (v *vtEmulator) resize(size Size) { v.emu.Resize(size.Cols, size.Rows) }

func (v *vtEmulator) defaults(fg, bg color.Color) {
	if fg != nil {
		v.emu.SetDefaultForegroundColor(fg)
	}
	if bg != nil {
		v.emu.SetDefaultBackgroundColor(bg)
	}
}

func (v *vtEmulator) replies() io.Reader { return v.emu }

func (v *vtEmulator) stopReplies() error { return v.pipe.Close() }

// look is the whole screen as the emulator holds it, for the tests that
// compare two emulators.
func (v *vtEmulator) look() look {
	size := Size{Cols: v.emu.Width(), Rows: v.emu.Height()}
	rows := make([][]Run, size.Rows)
	for y := range rows {
		rows[y] = v.row(y, size.Cols)
	}
	pos := v.emu.CursorPosition()
	modes := v.modes
	modes.AltScreen = v.emu.IsAltScreen()
	return look{
		size: size, rows: rows, cursor: Cursor{Row: pos.Y, Col: pos.X, Visible: !v.emu.CursorHidden()}, modes: modes,
		title: v.title,
	}
}

// row is one row as runs, walking the cells rather than parsing Render: the
// walk is the only way to the runs, and it is cheaper
// (docs/archive/v2/agent-deck-ideas.md, measured). A new run starts where the
// style changes; a wide glyph is a run of its own counted as its width, the
// cell after it, which only holds its place, skipped, so the preamble places
// the next run by the emulator's width, not a client's guess. A wide glyph
// cut by the right edge is drawn as a blank, so the runs add up to the width.
func (v *vtEmulator) row(y, width int) []Run {
	var runs []Run
	var text strings.Builder
	var open Run
	wide := false
	flush := func() {
		if open.Cols > 0 {
			open.Text = text.String()
			runs = append(runs, open)
		}
		text.Reset()
	}
	for x := 0; x < width; {
		cell := v.emu.CellAt(x, y)
		if cell != nil && cell.Width == 0 {
			x++
			continue
		}
		next := Run{Cols: 1}
		content := " "
		if cell != nil {
			next = Run{Cols: cell.Width, Fg: colorOf(cell.Style.Fg), Bg: colorOf(cell.Style.Bg), Attrs: attrsOf(cell.Style)}
			if cell.Content != "" {
				content = cell.Content
			}
			if cell.Style.Attrs&uv.AttrConceal != 0 {
				content = strings.Repeat(" ", next.Cols)
			}
		}
		if x+next.Cols > width {
			next.Cols, content = width-x, strings.Repeat(" ", width-x)
		}
		if open.Cols == 0 || next.Fg != open.Fg || next.Bg != open.Bg || next.Attrs != open.Attrs || wide || next.Cols > 1 {
			flush()
			open = Run{Fg: next.Fg, Bg: next.Bg, Attrs: next.Attrs}
		}
		wide = next.Cols > 1
		text.WriteString(content)
		open.Cols += next.Cols
		x += next.Cols
	}
	flush()
	return runs
}

// colorOf maps the library's colors: the basic and extended palettes to an
// index, anything else to its RGB, and a transparent one to the default.
func colorOf(c color.Color) Color {
	switch c := c.(type) {
	case nil:
		return Color{}
	case ansi.BasicColor:
		return Color{Kind: Indexed, Index: uint8(c)}
	case ansi.IndexedColor:
		return Color{Kind: Indexed, Index: uint8(c)}
	}
	r, g, b, a := c.RGBA()
	if a == 0 {
		return Color{}
	}
	return Color{Kind: RGB, R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8)}
}

func attrsOf(s uv.Style) Attrs {
	var a Attrs
	for _, pair := range []struct {
		lib  uint8
		mine Attrs
	}{{uv.AttrBold, Bold}, {uv.AttrFaint, Dim}, {uv.AttrItalic, Italic}, {uv.AttrReverse, Inverse}, {uv.AttrStrikethrough, Strikethrough}} {
		if s.Attrs&pair.lib != 0 {
			a |= pair.mine
		}
	}
	if s.Underline != uv.UnderlineNone {
		a |= Underline
	}
	return a
}
