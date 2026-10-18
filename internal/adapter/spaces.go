package adapter

import "context"

// What a main run's adapter shows of the spaces its run opened: the workers,
// each one's state, and a chosen worker's screen with its keys
// (docs/v2/workers-panel.md). The host composes it in, as it does a
// TurnHandler: an adapter never imports the host.

// SpacePanel is a run that shows its spaces to the person: the host hands
// it the run's SpaceView before the service starts, for a run whose role
// opens spaces.
type SpacePanel interface {
	ShowSpaces(SpaceView)
}

// SpaceView is what the host offers of the run's spaces.
type SpaceView interface {
	// Workers are the spaces this run opened that are not over, in the
	// order they opened.
	Workers() ([]SpaceWorker, error)
	// View connects to one space's screen; the caller closes it.
	View(space string) (ScreenView, error)
}

// SpaceWorker is one space's worker as the panel lists it. A field nobody
// observed is empty.
type SpaceWorker struct {
	// Space is the space's id, which View takes.
	Space   string `json:"space"`
	Name    string `json:"name"`
	Session string `json:"session"`
	Harness string `json:"harness"`
	// State is the space's: opening, open or closing.
	State string `json:"state"`
	// Activity is the worker's turn: working or idle.
	Activity string `json:"activity,omitempty"`
	Model    string `json:"model,omitempty"`
	Effort   string `json:"effort,omitempty"`
	// Percent is how full the worker's context is, -1 when unknown.
	Percent int `json:"percent"`
}

// ScreenView is one open view of a worker's screen.
type ScreenView interface {
	// Screen waits until the screen's sequence passes after, or ctx ends,
	// and answers the whole screen; Ended says the worker's output is over.
	Screen(ctx context.Context, after uint64) (Screen, error)
	// Keys writes bytes to the worker as typed.
	Keys([]byte) error
	// Resize sets the worker's terminal size.
	Resize(cols, rows int) error
	Close() error
}

// Screen is a worker's whole screen at one sequence.
type Screen struct {
	Seq    uint64
	Cols   int
	Rows   int
	Lines  [][]ScreenRun
	Cursor ScreenCursor
	Ended  bool
}

// ScreenRun is a stretch of a row in one style; Cols counts a wide glyph as
// two, and the runs of a row add up to its width.
type ScreenRun struct {
	Text  string
	Cols  int
	Fg    ScreenColor
	Bg    ScreenColor
	Attrs ScreenAttrs
}

// ScreenColor is a run's foreground or background: the terminal's default,
// a palette index, or a direct color.
type ScreenColor struct {
	Kind    ScreenColorKind
	Index   uint8
	R, G, B uint8
}

// ScreenColorKind tells a color's form.
type ScreenColorKind uint8

// The forms of a color.
const (
	DefaultColor ScreenColorKind = iota
	IndexedColor
	RGBColor
)

// ScreenAttrs are a run's attributes.
type ScreenAttrs uint8

// The attributes a run carries.
const (
	Bold ScreenAttrs = 1 << iota
	Dim
	Italic
	Underline
	Inverse
	Strikethrough
)

// ScreenCursor is where the cursor is, from 0, and whether it is shown.
type ScreenCursor struct {
	Row, Col int
	Visible  bool
}
