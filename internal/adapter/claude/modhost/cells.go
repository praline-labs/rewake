package modhost

import (
	"encoding/base64"
	"encoding/binary"
	"unicode"
	"unicode/utf8"

	"github.com/praline-labs/rewake/internal/adapter"
)

// A worker's screen as a Raster's cells (docs/v2/workers-panel.md): Claude
// Code draws a Raster from little-endian u32 triplets — a code point, a
// foreground and a background — and refuses the whole tree over one code
// point that is not a printable width-1 character of the BMP. So every cell
// is made one: a wide glyph is drawn as a mark in its first cell and a blank
// in its second, and anything else unprintable as the mark. A Raster has no
// attributes: inverse is drawn, the rest is lost.

const (
	// defaultColor is a Raster's terminal default.
	defaultColor uint32 = 0x01000000
	// lightGrey and darkGrey stand for the default foreground and background
	// in each other's place, inverse, where the default would be both.
	lightGrey uint32 = 0xd0d0d0
	darkGrey  uint32 = 0x1e1e1e
	// mark is drawn for a glyph a Raster cannot hold.
	mark = '?'
)

// cells is the screen's cells, base64 as a Raster takes them.
func cells(s adapter.Screen) string {
	data := make([]byte, 0, s.Cols*s.Rows*12)
	for row := range s.Rows {
		var line []adapter.ScreenRun
		if row < len(s.Lines) {
			line = s.Lines[row]
		}
		col := 0
		put := func(r rune, fg, bg uint32) {
			if col >= s.Cols {
				return
			}
			if s.Cursor.Visible && s.Cursor.Row == row && s.Cursor.Col == col {
				fg, bg = inverse(fg, bg)
			}
			data = binary.LittleEndian.AppendUint32(data, uint32(r))
			data = binary.LittleEndian.AppendUint32(data, fg)
			data = binary.LittleEndian.AppendUint32(data, bg)
			col++
		}
		for _, run := range line {
			fg, bg := rgbOf(run.Fg), rgbOf(run.Bg)
			if run.Attrs&adapter.Inverse != 0 {
				fg, bg = inverse(fg, bg)
			}
			for _, r := range glyphs(run.Text, run.Cols) {
				put(r, fg, bg)
			}
		}
		for col < s.Cols {
			put(' ', defaultColor, defaultColor)
		}
	}
	return base64.StdEncoding.EncodeToString(data)
}

// glyphs is a run's text as exactly cols code points of one cell each.
// Combining marks, joiners and variation selectors join the glyph before
// them and take no cell. The emulator gives a wide glyph a run of its own
// (terminal's row), so a run of one glyph over two columns is a wide glyph,
// drawn as the mark and a blank. A run of more glyphs than that whose
// columns do not match its glyphs is not the emulator's: none of its
// characters outside ASCII is trusted to be one cell wide, each is drawn as
// the mark, and what still does not add up is cut or padded with blanks, so
// the run keeps its width and the row stays aligned.
func glyphs(text string, cols int) []rune {
	var bases []rune
	for _, r := range text {
		if r == utf8.RuneError || joins(r) {
			continue
		}
		bases = append(bases, r)
	}
	out := make([]rune, 0, cols)
	switch {
	case len(bases) == 1 && cols == 2:
		out = append(out, mark, ' ')
	case len(bases) == cols:
		for _, r := range bases {
			out = append(out, single(r))
		}
	default:
		for _, r := range bases {
			if r >= 0x80 {
				r = mark
			}
			out = append(out, single(r))
		}
	}
	for len(out) < cols {
		out = append(out, ' ')
	}
	return out[:cols]
}

// joins says r joins the glyph before it rather than take a cell.
func joins(r rune) bool {
	return unicode.In(r, unicode.Mn, unicode.Me) || r == 0x200d || r >= 0xfe00 && r <= 0xfe0f || r >= 0xe0100 && r <= 0xe01ef
}

// single is r as a Raster may hold it: a printable character of the BMP
// outside the private use area, a space of any kind as the plain one, else
// the mark.
func single(r rune) rune {
	if r >= 0x20 && unicode.IsSpace(r) {
		return ' '
	}
	if r < 0x20 || r > 0xffff || r >= 0x7f && r < 0xa0 || r >= 0xe000 && r <= 0xf8ff || r >= 0xd800 && r <= 0xdfff || !unicode.IsPrint(r) && r != ' ' {
		return mark
	}
	return r
}

// inverse swaps a cell's colors; a default that would land in the other's
// place is drawn as the grey it usually is.
func inverse(fg, bg uint32) (uint32, uint32) {
	if fg == defaultColor {
		fg = lightGrey
	}
	if bg == defaultColor {
		bg = darkGrey
	}
	return bg, fg
}

func rgbOf(c adapter.ScreenColor) uint32 {
	switch c.Kind {
	case adapter.IndexedColor:
		return palette(c.Index)
	case adapter.RGBColor:
		return uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B)
	}
	return defaultColor
}

// basic are xterm's sixteen colors.
var basic = [16]uint32{
	0x000000, 0xcd0000, 0x00cd00, 0xcdcd00, 0x0000ee, 0xcd00cd, 0x00cdcd, 0xe5e5e5,
	0x7f7f7f, 0xff0000, 0x00ff00, 0xffff00, 0x5c5cff, 0xff00ff, 0x00ffff, 0xffffff,
}

// palette is xterm's 256-color palette: the sixteen, a 6×6×6 cube, 24 greys.
func palette(index uint8) uint32 {
	switch {
	case index < 16:
		return basic[index]
	case index < 232:
		level := func(v uint8) uint32 {
			if v == 0 {
				return 0
			}
			return uint32(55 + 40*int(v))
		}
		i := index - 16
		return level(i/36)<<16 | level(i/6%6)<<8 | level(i%6)
	}
	grey := uint32(8 + 10*int(index-232))
	return grey<<16 | grey<<8 | grey
}
