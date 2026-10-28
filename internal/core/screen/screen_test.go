package screen

import (
	"bytes"
	"image/png"
	"strings"
	"testing"
)

func sample() *Frame {
	return &Frame{
		Source: SourceShell, Cols: 12, Rows: 2,
		Cursor: &Cursor{X: 0, Y: 1},
		Lines: [][]Run{
			{{T: "┌─┐", W: 3}, {T: "世界", W: 4, Fg: "#ff0000", Bg: "#0000ff", A: Bold}, {T: "ok  ", W: 4}},
			{{T: "x", W: 1}},
		},
	}
}

func TestTextKeepsWideGlyphsAndTrimsBlanks(t *testing.T) {
	got := sample().Text()
	if got[0] != "┌─┐世界ok" || got[1] != "x" {
		t.Fatalf("text = %q", got)
	}
}

func TestPNGDrawsTheCellsAtTheirPlace(t *testing.T) {
	data, err := sample().PNG()
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 12*cellW || b.Dy() != 2*cellH {
		t.Fatalf("size = %v", b)
	}
	// The run on a blue background starts at column 3.
	r, g, bl, _ := img.At(3*cellW, 0).RGBA()
	if r != 0 || g != 0 || bl>>8 != 0xff {
		t.Fatalf("pixel of the blue run = %x %x %x", r, g, bl)
	}
	// The cursor inverts its cell: the default background becomes light.
	if r, _, _, _ := img.At(cellW-1, cellH+cellH-1).RGBA(); r>>8 < 0xd0 {
		t.Fatalf("cursor cell not inverted: %x", r)
	}
}

func TestTheFontHasBoxDrawingAndWideGlyphs(t *testing.T) {
	if g, ok := glyphOf('─'); !ok || g.wide {
		t.Fatalf("box drawing: %v %v", g.wide, ok)
	}
	if g, ok := glyphOf('世'); !ok || !g.wide {
		t.Fatalf("wide glyph: %v %v", g.wide, ok)
	}
}

func TestCheckRefusesAFrameOutsideItsBounds(t *testing.T) {
	for name, f := range map[string]*Frame{
		"too wide":   {Cols: MaxCols + 1, Rows: 1, Lines: [][]Run{nil}},
		"no rows":    {Cols: 1, Rows: 0},
		"few lines":  {Cols: 1, Rows: 2, Lines: [][]Run{nil}},
		"long line":  {Cols: 2, Rows: 1, Lines: [][]Run{{{T: "abc", W: 3}}}},
		"cursor out": {Cols: 2, Rows: 1, Lines: [][]Run{nil}, Cursor: &Cursor{X: 2}},
	} {
		if err := f.Check(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := (&Frame{Cols: 1, Rows: 2}).PNG(); err == nil || !strings.Contains(err.Error(), "lines") {
		t.Fatalf("PNG of a broken frame: %v", err)
	}
}
