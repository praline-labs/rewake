package terminal

// The screen as the emulator holds it: rows of styled runs, which a late
// client's preamble draws (replay.go), and the input modes it sets. This
// package knows no harness and no mod.

// Run is a stretch of a row in one style. Cols counts a wide glyph as two;
// the runs of a row add up to its width.
type Run struct {
	Text  string
	Cols  int
	Fg    Color
	Bg    Color
	Attrs Attrs
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
	Kind  ColorKind
	Index uint8
	R     uint8
	G     uint8
	B     uint8
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
	Row     int
	Col     int
	Visible bool
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
// as: a client without them sends Right to a program in application cursor
// mode as CSI C where it waits for SS3 C.
type Modes struct {
	// CursorKeys is DECCKM (mode 1): arrows, Home and End as SS3 sequences.
	CursorKeys bool
	// Keypad is DECNKM (mode 66): the keypad in application mode.
	Keypad bool
	// BracketedPaste is mode 2004: a paste framed by CSI 200~ and 201~.
	BracketedPaste bool
	Mouse          MouseMode
	// MouseSGR is mode 1006: mouse reports in the SGR form.
	MouseSGR bool
	// Focus is mode 1004: focus in and out reported.
	Focus bool
	// KittyKeys are the kitty keyboard protocol's flags in force (CSI > u):
	// 1 disambiguates, so Shift+Enter differs from Enter.
	KittyKeys uint8
	// ModifyOtherKeys is xterm's modifyOtherKeys level, 0 to 2.
	ModifyOtherKeys uint8
	// AltScreen says the alternate screen is shown.
	AltScreen bool
}

// look is the whole screen as the emulator holds it at one moment.
type look struct {
	size   Size
	rows   [][]Run
	cursor Cursor
	modes  Modes
	title  string
}
