package terminal

import (
	"strconv"
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
)

// replay is the state as bytes a fresh terminal takes to stand where this
// emulator stands (docs/shell.md): both screens with their margins, saved
// cursors and cursors (checkpoint.go), a pending wrap, the title, and every
// mode. A late client without the input modes loses Shift+Enter; a next
// image without the rest writes the program's later output elsewhere.
//
// A seed also carries the modes only the emulator acts on, grapheme
// clustering and in-band size reports, which a client's terminal is not
// given: under ?2048 it would report its own size to the program.
func (v *vtEmulator) replay(seed bool) []byte {
	c := v.checkpoint()
	var b strings.Builder
	b.WriteString("\x1b[0m\x1b[H\x1b[2J")
	// DECSLRM is taken only under DECLRMM, and the library keeps the left
	// and right margins after DECLRMM is reset, so both are written and
	// DECLRMM is reset afterwards when the program had.
	b.WriteString("\x1b[?69h")
	main, alt := c.screens[0], c.screens[1]
	v.screenReplay(&b, main, true)
	// The alternate screen is entered by 1047, which saves no cursor, so the
	// main screen's saved cursor stays the one just set. Hidden, it is
	// entered only for the margins and saved cursor the library keeps for
	// its next entry.
	if c.alt {
		cursorReplay(&b, main.cursor, uv.Rectangle{})
		b.WriteString("\x1b[?1047h")
		v.screenReplay(&b, alt, true)
	} else {
		b.WriteString("\x1b[?1047h")
		v.screenReplay(&b, alt, false)
		b.WriteString("\x1b[?1047l")
	}
	if !v.modes.Margins {
		b.WriteString("\x1b[?69l")
	}
	active := c.active()
	placed := func(origin uv.Rectangle) {
		cursorReplay(&b, active.cursor, origin)
		if c.phantom {
			pendingWrap(&b, active, origin)
		}
	}
	switch {
	case !v.modes.Origin:
		placed(uv.Rectangle{})
	case active.cursor.In(active.scroll):
		// DECOM before the cursor, which is then placed within the margins:
		// a terminal homes the cursor when DECOM is set.
		b.WriteString("\x1b[?6h")
		placed(active.scroll)
	default:
		// Under DECOM no CUP reaches a cursor outside the margins. It is
		// placed before, which the library keeps and a terminal that homes
		// the cursor on DECOM does not, until the program moves it.
		placed(uv.Rectangle{})
		b.WriteString("\x1b[?6h")
	}
	if v.modes.NoWrap {
		// After the pending wrap, which only autowrap makes.
		b.WriteString("\x1b[?7l")
	}
	if v.modes.NewLine {
		b.WriteString("\x1b[20h")
	}
	if v.title != "" {
		b.WriteString("\x1b]2;" + v.title + "\x07")
	}
	b.WriteString(v.modes.replay())
	if seed && v.modes.Unicode {
		b.WriteString("\x1b[?2027h")
	}
	if seed && v.modes.InBandResize {
		b.WriteString("\x1b[?2048h")
	}
	b.WriteString(v.kittyReplay())
	return []byte(b.String())
}

// screenReplay writes the shown screen's cells when cells is set, then its
// margins and its saved cursor. The cells go first: margins would bend
// their lines.
func (v *vtEmulator) screenReplay(b *strings.Builder, s screenState, cells bool) {
	size := v.size()
	if cells {
		for y := range size.Rows {
			col := 0
			for _, run := range rowOf(s.screen.CellAt, y, size.Cols) {
				if run.Fg != (Color{}) || run.Bg != (Color{}) || run.Attrs != 0 || strings.TrimRight(run.Text, " ") != "" {
					// Each run is placed, so a client measuring a glyph's
					// width otherwise than this emulator is off by a run
					// at most.
					b.WriteString("\x1b[" + strconv.Itoa(y+1) + ";" + strconv.Itoa(col+1) + "H")
					b.WriteString(sgr(run))
					b.WriteString(run.Text)
				}
				col += run.Cols
			}
		}
	}
	// DECSTBM and DECSLRM; each homes the cursor.
	b.WriteString("\x1b[" + strconv.Itoa(s.scroll.Min.Y+1) + ";" + strconv.Itoa(s.scroll.Max.Y) + "r")
	b.WriteString("\x1b[" + strconv.Itoa(s.scroll.Min.X+1) + ";" + strconv.Itoa(s.scroll.Max.X) + "s")
	cursorReplay(b, s.saved, uv.Rectangle{})
	b.WriteString("\x1b7")
}

// cursorReplay places the cursor, within origin when DECOM is set, and
// gives it its pen, its style and its visibility, all of which DECSC saves.
func cursorReplay(b *strings.Builder, cur vt.Cursor, origin uv.Rectangle) {
	b.WriteString("\x1b[" + strconv.Itoa(cur.Y-origin.Min.Y+1) + ";" + strconv.Itoa(cur.X-origin.Min.X+1) + "H")
	b.WriteString(penReplay(cur.Pen))
	shape := 2*int(cur.Style) + 1
	if cur.Steady {
		shape++
	}
	b.WriteString("\x1b[" + strconv.Itoa(shape) + " q")
	if cur.Hidden {
		b.WriteString("\x1b[?25l")
	} else {
		b.WriteString("\x1b[?25h")
	}
}

// pendingWrap writes the last cell of the cursor's row again in its own
// style, which leaves the cursor pending a wrap as the program left it: the
// next glyph begins the next line rather than overwrite this one. Then the
// pen again. The second half of a wide glyph is left as it is.
func pendingWrap(b *strings.Builder, s screenState, origin uv.Rectangle) {
	width := s.screen.Width()
	cell := s.screen.CellAt(width-1, s.cursor.Y)
	if cell == nil || cell.Width == 0 {
		return
	}
	content := cell.Content
	if content == "" {
		content = " "
	}
	b.WriteString("\x1b[" + strconv.Itoa(s.cursor.Y-origin.Min.Y+1) + ";" + strconv.Itoa(width-origin.Min.X) + "H")
	b.WriteString(penReplay(cell.Style) + content + penReplay(s.cursor.Pen))
}

// penReplay is a pen whole, from the defaults.
func penReplay(pen uv.Style) string {
	if pen.IsZero() {
		return "\x1b[0m"
	}
	return "\x1b[0m" + pen.String()
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
