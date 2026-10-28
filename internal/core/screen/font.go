package screen

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed" // the font
	"encoding/hex"
	"strconv"
	"sync"
)

// The font is GNU Unifont 16.0.01's Basic Multilingual Plane, a bitmap
// font drawn on a cell of 8 by 16 pixels, a wide glyph on 16 by 16: it
// ships in the binary, so no system font is assumed, and it holds the box
// drawing and the wide glyphs a terminal shows. It is used under the SIL Open
// Font License 1.1 (https://unifoundry.com/unifont/), copyright the Unifont
// authors; the file is the project's own .hex, kept to the plane, gzipped.
//
//go:embed unifont-bmp.hex.gz
var unifontGz []byte

// glyph is one character's bitmap, a row per two bytes when wide.
type glyph struct {
	wide bool
	rows []byte
}

var (
	fontOnce sync.Once
	fontMap  map[rune]glyph
)

// glyphOf is the font's glyph for r, if it has one.
func glyphOf(r rune) (glyph, bool) {
	fontOnce.Do(loadFont)
	g, ok := fontMap[r]
	return g, ok
}

func loadFont() {
	fontMap = make(map[rune]glyph, 57000)
	zr, err := gzip.NewReader(bytes.NewReader(unifontGz))
	if err != nil {
		return
	}
	defer func() { _ = zr.Close() }()
	scan := bufio.NewScanner(zr)
	for scan.Scan() {
		line := scan.Bytes()
		colon := bytes.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		code, err := strconv.ParseUint(string(line[:colon]), 16, 32)
		if err != nil {
			continue
		}
		rows := make([]byte, (len(line)-colon-1)/2)
		if _, err := hex.Decode(rows, line[colon+1:]); err != nil {
			continue
		}
		fontMap[rune(code)] = glyph{wide: len(rows) == 32, rows: rows}
	}
}

// at is whether the glyph has the pixel x on row y.
func (g glyph) at(x, y int) bool {
	if g.wide {
		return g.rows[2*y+x/8]&(0x80>>(x%8)) != 0
	}
	return g.rows[y]&(0x80>>x) != 0
}
