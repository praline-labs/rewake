package terminal

import "slices"

// The screen as the host mode shows it (docs/v2/host-mode.md): rows of
// styled runs with the cursor, and the input modes the person's terminal
// mirrors. This package knows no harness and no mod.

// Screen is the screen at one sequence, carrying the rows changed since the
// sequence its reader had.
type Screen struct {
	// Seq is this screen's sequence: the after of the next read.
	Seq  uint64 `json:"seq"`
	Size Size   `json:"size"`
	// Changed are the rows changed since the reader's sequence, every row
	// when it had none or had one this terminal never gave.
	Changed []Row  `json:"changed"`
	Cursor  Cursor `json:"cursor"`
	Modes   Modes  `json:"modes"`
	// Ended says the program's side is closed: the screen changes no more.
	Ended bool `json:"ended,omitempty"`
}

// Row is one row as runs that cover its width, left to right.
type Row struct {
	Index int   `json:"row"`
	Runs  []Run `json:"runs"`
}

// Run is a stretch of a row in one style. Cols counts a wide glyph as two;
// the runs of a row add up to its width.
type Run struct {
	Text  string `json:"text"`
	Cols  int    `json:"cols"`
	Fg    Color  `json:"fg"`
	Bg    Color  `json:"bg"`
	Attrs Attrs  `json:"attrs,omitempty"`
}

// ColorKind tells a color's form.
type ColorKind uint8

// The forms of a color. Default is the terminal's own, whatever the mod's
// theme makes it.
const (
	Default ColorKind = iota
	Indexed
	RGB
)

// Color is a run's foreground or background: the default, a palette index
// (0–15 the basic colors, up to 255 the extended), or a direct color.
type Color struct {
	Kind  ColorKind `json:"kind"`
	Index uint8     `json:"index,omitempty"`
	R     uint8     `json:"r,omitempty"`
	G     uint8     `json:"g,omitempty"`
	B     uint8     `json:"b,omitempty"`
}

// Attrs are a run's attributes. Blink has no place in the mod's text and is
// dropped; concealed text is drawn as blanks, since it is hidden on purpose.
type Attrs uint8

// The attributes a run carries.
const (
	Bold Attrs = 1 << iota
	Dim
	Italic
	Underline
	Inverse
	Strikethrough
)

// Cursor is where the cursor is, from 0, and whether the program shows it.
type Cursor struct {
	Row     int  `json:"row"`
	Col     int  `json:"col"`
	Visible bool `json:"visible"`
}

// MouseMode is which mouse events the program asked for.
type MouseMode uint8

// The mouse modes, by the DEC private mode that sets each.
const (
	MouseOff    MouseMode = iota
	MouseX10              // 9: presses only
	MouseNormal           // 1000: presses and releases
	MouseButton           // 1002: and motion while a button is down
	MouseAny              // 1003: and all motion
)

// Modes are what the program set that changes the bytes a key must be sent
// as: the encoder in the mod reads them, or Right reaches a program in
// application cursor mode as CSI C where it waits for SS3 C.
type Modes struct {
	// CursorKeys is DECCKM (mode 1): arrows, Home and End as SS3 sequences.
	CursorKeys bool `json:"cursorKeys,omitempty"`
	// Keypad is DECNKM (mode 66): the keypad in application mode.
	Keypad bool `json:"keypad,omitempty"`
	// BracketedPaste is mode 2004: a paste framed by CSI 200~ and 201~.
	BracketedPaste bool      `json:"bracketedPaste,omitempty"`
	Mouse          MouseMode `json:"mouse,omitempty"`
	// MouseSGR is mode 1006: mouse reports in the SGR form.
	MouseSGR bool `json:"mouseSgr,omitempty"`
	// Focus is mode 1004: focus in and out reported.
	Focus bool `json:"focus,omitempty"`
	// AltScreen says the alternate screen is shown.
	AltScreen bool `json:"altScreen,omitempty"`
}

// look is the whole screen as the emulator holds it at one moment.
type look struct {
	size   Size
	rows   [][]Run
	cursor Cursor
	modes  Modes
}

// published is what the terminal last gave out: the rows, and the sequence
// at which each last changed, so a read after any sequence gets exactly the
// rows changed since.
type published struct {
	seq     uint64
	size    Size
	rows    [][]Run
	changed []uint64
	cursor  Cursor
	modes   Modes
}

// take compares a fresh look with what was published and moves the
// sequence when anything differs — a row, the size, the cursor, a mode. A
// new size republishes every row, since the mod's rows no longer fit.
func (p *published) take(l look) {
	resized := l.size != p.size || len(p.rows) != len(l.rows)
	moved := l.cursor != p.cursor || l.modes != p.modes
	next := p.seq + 1
	differs := resized || moved
	if resized {
		p.changed = make([]uint64, len(l.rows))
		for i := range p.changed {
			p.changed[i] = next
		}
	} else {
		for i, row := range l.rows {
			if !slices.Equal(row, p.rows[i]) {
				p.changed[i] = next
				differs = true
			}
		}
	}
	if !differs {
		return
	}
	p.seq, p.size, p.rows, p.cursor, p.modes = next, l.size, l.rows, l.cursor, l.modes
}

// since is the screen for a reader that has after: the rows changed since,
// or every row when after is 0 or a sequence this terminal never reached.
func (p *published) since(after uint64) Screen {
	s := Screen{Seq: p.seq, Size: p.size, Cursor: p.cursor, Modes: p.modes, Changed: []Row{}}
	all := after == 0 || after > p.seq
	for i, row := range p.rows {
		if all || p.changed[i] > after {
			s.Changed = append(s.Changed, Row{Index: i, Runs: row})
		}
	}
	return s
}
