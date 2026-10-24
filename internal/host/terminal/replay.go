package terminal

import (
	"strconv"
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// replay is the screen and the input modes as bytes a fresh terminal takes
// to stand where this emulator stands (docs/shell.md): the alternate
// screen, the screen's cells, the cursor, the title, and every mode a key's
// bytes depend on. A late client without the modes loses Shift+Enter.
func (v *vtEmulator) replay() []byte {
	var b strings.Builder
	alt := v.emu.IsAltScreen()
	if alt {
		b.WriteString("\x1b[?1049h")
	}
	b.WriteString("\x1b[0m\x1b[H\x1b[2J")
	cols, rows := v.emu.Width(), v.emu.Height()
	for y := range rows {
		col := 0
		for _, run := range v.row(y, cols) {
			if run.Fg != (Color{}) || run.Bg != (Color{}) || run.Attrs != 0 || strings.TrimRight(run.Text, " ") != "" {
				// Each run is placed, so a client measuring a glyph's width
				// otherwise than this emulator is off by a run at most.
				b.WriteString("\x1b[" + strconv.Itoa(y+1) + ";" + strconv.Itoa(col+1) + "H")
				b.WriteString(sgr(run))
				b.WriteString(run.Text)
			}
			col += run.Cols
		}
	}
	b.WriteString(sgr(Run{Fg: colorOf(v.pen.Fg), Bg: colorOf(v.pen.Bg), Attrs: attrsOf(v.pen)}))
	pos := v.emu.CursorPosition()
	b.WriteString("\x1b[" + strconv.Itoa(pos.Y+1) + ";" + strconv.Itoa(pos.X+1) + "H")
	if v.emu.CursorHidden() {
		b.WriteString("\x1b[?25l")
	} else {
		b.WriteString("\x1b[?25h")
	}
	if v.title != "" {
		b.WriteString("\x1b]2;" + v.title + "\x07")
	}
	b.WriteString(v.modes.replay())
	b.WriteString(v.kittyReplay())
	return []byte(b.String())
}

// followPen keeps the pen beside the library's: a cut between an SGR and
// the text after it is common in a pty read, and a client without the pen
// writes that text plain. A pen restored by DECRC is not followed; the next
// SGR sets it right.
func (v *vtEmulator) followPen() {
	// Called before the library's own handlers, which still run.
	v.emu.RegisterCsiHandler('m', func(p ansi.Params) bool {
		uv.ReadStyle(p, &v.pen)
		return false
	})
	v.emu.RegisterEscHandler('c', func() bool {
		v.pen = uv.Style{}
		return false
	})
}

// replay sets the modes that are on; a fresh terminal has every one off.
func (m Modes) replay() string {
	var b strings.Builder
	set := func(on bool, mode string) {
		if on {
			b.WriteString("\x1b[?" + mode + "h")
		}
	}
	set(m.CursorKeys, "1")
	set(m.Keypad, "66")
	set(m.BracketedPaste, "2004")
	set(m.Focus, "1004")
	set(m.Mouse == MouseX10, "9")
	set(m.Mouse == MouseNormal, "1000")
	set(m.Mouse == MouseButton, "1002")
	set(m.Mouse == MouseAny, "1003")
	set(m.MouseSGR, "1006")
	if m.ModifyOtherKeys > 0 {
		b.WriteString("\x1b[>4;" + strconv.Itoa(int(m.ModifyOtherKeys)) + "m")
	}
	return b.String()
}

// kittyReplay rebuilds the kitty keyboard stack: the bottom entry set, then
// one push per entry above it in order, the flags in force last, so a pop
// the program makes later finds what it pushed before.
func (v *vtEmulator) kittyReplay() string {
	entries := append(append([]uint8(nil), v.kitty...), v.modes.KittyKeys)
	var b strings.Builder
	if entries[0] != 0 {
		b.WriteString("\x1b[=" + strconv.Itoa(int(entries[0])) + ";1u")
	}
	for _, flags := range entries[1:] {
		b.WriteString("\x1b[>" + strconv.Itoa(int(flags)) + "u")
	}
	return b.String()
}

// sgr is the run's style from the defaults.
func sgr(run Run) string {
	params := []string{"0"}
	for _, pair := range []struct {
		attr  Attrs
		param string
	}{{Bold, "1"}, {Dim, "2"}, {Italic, "3"}, {Underline, "4"}, {Inverse, "7"}, {Strikethrough, "9"}} {
		if run.Attrs&pair.attr != 0 {
			params = append(params, pair.param)
		}
	}
	params = append(params, colorParams(run.Fg, 30, 90, "38")...)
	params = append(params, colorParams(run.Bg, 40, 100, "48")...)
	return "\x1b[" + strings.Join(params, ";") + "m"
}

// colorParams keeps an index as an index, so the client draws it in the
// palette it was given.
func colorParams(c Color, basic, bright int, extended string) []string {
	switch c.Kind {
	case Indexed:
		switch {
		case c.Index < 8:
			return []string{strconv.Itoa(basic + int(c.Index))}
		case c.Index < 16:
			return []string{strconv.Itoa(bright + int(c.Index) - 8)}
		}
		return []string{extended, "5", strconv.Itoa(int(c.Index))}
	case RGB:
		return []string{extended, "2", strconv.Itoa(int(c.R)), strconv.Itoa(int(c.G)), strconv.Itoa(int(c.B))}
	}
	return nil
}
