package terminal

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image/color"
	"io"
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
)

// maxClipboard is the most base64 an OSC 52 may carry to be taken: the
// library bounds a string at 4 MiB, but decoding a large one costs several
// copies, and a clipboard of more than this is not text a person pastes.
const maxClipboard = 1 << 20

// vtEmulator is the emulator of github.com/charmbracelet/x/vt, pinned at
// v0.1.0: the pseudo-version agent-deck pins loses a combining mark after an
// ASCII base within one write (docs/v2/agent-deck-ideas.md).
type vtEmulator struct {
	emu *vt.Emulator
	// pipe is the write end of the library's reply pipe, closed to end the
	// reply reader. Closing the emulator would do it too, but Close writes a
	// field Read reads without a lock, a race the detector reports.
	pipe io.Closer
	// modes are kept from the mode callbacks, which run inside write, so
	// they change only under the terminal's lock.
	modes Modes
}

func newVT(size Size, clipboard func([]byte), refused func()) (emulator, error) {
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
	emu.RegisterOscHandler(52, func(data []byte) bool {
		if payload, ok := clipboardPayload(data); ok {
			clipboard(payload)
		} else {
			refused()
		}
		return true
	})
	return v, nil
}

// clipboardPayload reads OSC 52's "52;<selection>;<base64>". A query, "?",
// is refused rather than answered: there is no clipboard here to read, and
// an answer would hand the program whatever it was told last.
func clipboardPayload(data []byte) ([]byte, bool) {
	parts := bytes.SplitN(data, []byte{';'}, 3)
	if len(parts) != 3 || len(parts[2]) == 0 || len(parts[2]) > maxClipboard || string(parts[2]) == "?" {
		return nil, false
	}
	payload, err := base64.StdEncoding.DecodeString(string(parts[2]))
	if err != nil {
		return nil, false
	}
	return payload, true
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

func (v *vtEmulator) write(p []byte) { _, _ = v.emu.Write(p) }

func (v *vtEmulator) resize(size Size) { v.emu.Resize(size.Cols, size.Rows) }

func (v *vtEmulator) replies() io.Reader { return v.emu }

func (v *vtEmulator) stopReplies() error { return v.pipe.Close() }

// look walks the cells rather than parsing Render: the walk is the only way
// to the runs, and it is cheaper (docs/v2/agent-deck-ideas.md, measured).
func (v *vtEmulator) look() look {
	size := Size{Cols: v.emu.Width(), Rows: v.emu.Height()}
	rows := make([][]Run, size.Rows)
	for y := range rows {
		rows[y] = v.row(y, size.Cols)
	}
	pos := v.emu.CursorPosition()
	modes := v.modes
	modes.AltScreen = v.emu.IsAltScreen()
	return look{size: size, rows: rows, cursor: Cursor{Row: pos.Y, Col: pos.X, Visible: !v.emu.CursorHidden()}, modes: modes}
}

// row is one row as runs: a new run where the style changes, a wide glyph
// counted as its width, the cell after it, which only holds its place,
// skipped. A wide glyph cut by the right edge is drawn as a blank, so the
// runs add up to the width.
func (v *vtEmulator) row(y, width int) []Run {
	var runs []Run
	var text strings.Builder
	var open Run
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
		if open.Cols == 0 || next.Fg != open.Fg || next.Bg != open.Bg || next.Attrs != open.Attrs {
			flush()
			open = Run{Fg: next.Fg, Bg: next.Bg, Attrs: next.Attrs}
		}
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
