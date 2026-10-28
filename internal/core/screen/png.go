package screen

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strconv"
	"unicode"
)

// The cell of the picture, in pixels.
const (
	cellW = 8
	cellH = 16
)

// The colors a frame that names none is drawn in.
var (
	defaultFg = color.RGBA{0xd8, 0xd8, 0xd8, 0xff}
	defaultBg = color.RGBA{0x14, 0x14, 0x14, 0xff}
)

// PNG draws the screen. A glyph the font lacks is a hollow box, a wide one
// two cells; a mark that combines is drawn over the cell before it.
func (f *Frame) PNG() ([]byte, error) {
	if err := f.Check(); err != nil {
		return nil, err
	}
	fg, bg := colorOf(f.Fg, defaultFg), colorOf(f.Bg, defaultBg)
	img := image.NewRGBA(image.Rect(0, 0, f.Cols*cellW, f.Rows*cellH))
	fill(img, 0, 0, f.Cols*cellW, f.Rows*cellH, bg)
	for y, line := range f.Lines {
		x := 0
		for _, run := range line {
			drawRun(img, x, y, run, fg, bg)
			x += run.W
		}
	}
	if c := f.Cursor; c != nil {
		invert(img, c.X*cellW, c.Y*cellH, cellW, cellH)
	}
	var out bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func drawRun(img *image.RGBA, col, row int, run Run, fg, bg color.RGBA) {
	rfg, rbg := colorOf(run.Fg, fg), colorOf(run.Bg, bg)
	if run.A&Inverse != 0 {
		rfg, rbg = rbg, rfg
	}
	if run.A&Dim != 0 {
		rfg = mix(rfg, rbg)
	}
	px, py, w := col*cellW, row*cellH, run.W*cellW
	fill(img, px, py, w, cellH, rbg)
	bold := run.A&Bold != 0
	x, last := px, px
	for _, r := range run.T {
		g, ok := glyphOf(r)
		if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) {
			if ok {
				blit(img, last, py, g, rfg, bold, px+w)
			}
			continue
		}
		width := cellW
		if ok && g.wide {
			width = 2 * cellW
		}
		if x+width > px+w {
			break
		}
		last = x
		switch {
		case r == ' ' || r == 0:
		case ok:
			blit(img, x, py, g, rfg, bold, px+w)
		default:
			box(img, x, py, width, rfg)
		}
		x += width
	}
	if run.A&Underline != 0 {
		fill(img, px, py+cellH-2, w, 1, rfg)
	}
	if run.A&Strike != 0 {
		fill(img, px, py+cellH/2, w, 1, rfg)
	}
}

// blit draws a glyph, twice a pixel apart when bold, never past limit.
func blit(img *image.RGBA, x0, y0 int, g glyph, c color.RGBA, bold bool, limit int) {
	width := cellW
	if g.wide {
		width = 2 * cellW
	}
	for y := 0; y < cellH; y++ {
		for x := 0; x < width; x++ {
			if !g.at(x, y) {
				continue
			}
			set(img, x0+x, y0+y, c, limit)
			if bold {
				set(img, x0+x+1, y0+y, c, limit)
			}
		}
	}
}

func box(img *image.RGBA, x, y, w int, c color.RGBA) {
	for i := 1; i < w-1; i++ {
		set(img, x+i, y+2, c, x+w)
		set(img, x+i, y+cellH-3, c, x+w)
	}
	for j := 2; j < cellH-2; j++ {
		set(img, x+1, y+j, c, x+w)
		set(img, x+w-2, y+j, c, x+w)
	}
}

func set(img *image.RGBA, x, y int, c color.RGBA, limit int) {
	if x < limit && image.Pt(x, y).In(img.Rect) {
		img.SetRGBA(x, y, c)
	}
}

func fill(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			set(img, i, j, c, x+w)
		}
	}
}

func invert(img *image.RGBA, x, y, w, h int) {
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			c := img.RGBAAt(i, j)
			img.SetRGBA(i, j, color.RGBA{255 - c.R, 255 - c.G, 255 - c.B, 255})
		}
	}
}

func mix(a, b color.RGBA) color.RGBA {
	return color.RGBA{uint8((int(a.R) + int(b.R)) / 2), uint8((int(a.G) + int(b.G)) / 2), uint8((int(a.B) + int(b.B)) / 2), 255}
}

// colorOf parses "#rrggbb"; anything else is the fallback.
func colorOf(s string, fallback color.RGBA) color.RGBA {
	if len(s) != 7 || s[0] != '#' {
		return fallback
	}
	n, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return fallback
	}
	return color.RGBA{uint8(n >> 16), uint8(n >> 8), uint8(n), 255}
}
