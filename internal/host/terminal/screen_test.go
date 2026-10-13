package terminal

import (
	"testing"
)

// emulated is the library's emulator alone, with no terminal around it.
func emulated(t *testing.T, size Size) *vtEmulator {
	t.Helper()
	e, err := newVT(size, func([]byte) {}, func() {})
	if err != nil {
		t.Fatal(err)
	}
	return e.(*vtEmulator)
}

// fed writes chunks into a fresh emulator as the terminal's feeder does,
// holding the last cluster of each, and looks at the result.
func fed(t *testing.T, size Size, chunks ...string) look {
	t.Helper()
	e := emulated(t, size)
	var h holder
	for _, chunk := range chunks {
		e.write(h.take([]byte(chunk)))
	}
	e.write(h.release())
	return e.look()
}

// A glyph cluster split across two reads at any byte draws as it does whole:
// a combining mark, a variation selector, a joiner, a keycap, a conjunct and
// a flag, each split at every offset.
func TestAClusterSplitAcrossReadsDrawsAsWhole(t *testing.T) {
	size := Size{Cols: 12, Rows: 1}
	for _, text := range []string{
		"e\u0301x", "\u2764\ufe0fx", "\U0001F469\u200d\U0001F4BBx", "1\ufe0f\u20e3x",
		"\u0915\u094d\u0937x", "\U0001F1FA\U0001F1F8x", "ab\u0301",
	} {
		whole := fed(t, size, text)
		for i := 1; i < len(text); i++ {
			if split := fed(t, size, text[:i], text[i:]); !equalRuns(split.rows[0], whole.rows[0]) {
				t.Errorf("%+q split at %d:\n got %+v\nwant %+v", text, i, split.rows[0], whole.rows[0])
			}
		}
	}
}

// The library ends a cluster at the end of every write, which is why the
// feeder holds the last one. When an update stops doing so, this fails, and
// the hold may go.
func TestTheLibraryEndsAClusterAtTheEndOfAWrite(t *testing.T) {
	e := emulated(t, Size{Cols: 4, Rows: 1})
	e.write([]byte("e"))
	e.write([]byte("\u0301"))
	if got := e.emu.CellAt(0, 0).Content; got == "e\u0301" {
		t.Fatal("the library now joins a mark written after its base: reconsider the hold in hold.go")
	}
}

func TestHeldFromHoldsOneClusterOfText(t *testing.T) {
	for _, c := range []struct {
		data string
		held string
	}{
		{"abc", "c"},
		{"abc\r\n", ""},
		{"a\x1b[31", "1"},
		{"e\u0301", "e\u0301"},
		{"x\xe2\x9d", "x\xe2\x9d"},
		{"", ""},
		{string(make([]byte, 100)) + "\u0301\u0301", "\u0301\u0301"},
	} {
		if got := c.data[heldFrom([]byte(c.data)):]; got != c.held {
			t.Errorf("%+q holds %+q, want %+q", c.data, got, c.held)
		}
	}
	long := "a"
	for range holdWindow {
		long += "\u0301"
	}
	if got := heldFrom([]byte(long)); got != len(long) {
		t.Errorf("a cluster longer than the window is held from %d", got)
	}
}

// Each attribute and color form maps to its run, a wide glyph counts two
// columns, concealed text is blank, blink and links are dropped, and a row's
// runs add up to its width.
func TestCellsBecomeRuns(t *testing.T) {
	idx := func(i uint8) Color { return Color{Kind: Indexed, Index: i} }
	for _, c := range []struct {
		name  string
		input string
		first []Run
	}{
		{"bold", "\x1b[1ma", []Run{{Text: "a", Cols: 1, Attrs: Bold}}},
		{"dim", "\x1b[2ma", []Run{{Text: "a", Cols: 1, Attrs: Dim}}},
		{"italic", "\x1b[3ma", []Run{{Text: "a", Cols: 1, Attrs: Italic}}},
		{"underline", "\x1b[4ma", []Run{{Text: "a", Cols: 1, Attrs: Underline}}},
		{"curly underline", "\x1b[4:3ma", []Run{{Text: "a", Cols: 1, Attrs: Underline}}},
		{"inverse", "\x1b[7ma", []Run{{Text: "a", Cols: 1, Attrs: Inverse}}},
		{"strikethrough", "\x1b[9ma", []Run{{Text: "a", Cols: 1, Attrs: Strikethrough}}},
		{"combined", "\x1b[1;3;4;7ma", []Run{{Text: "a", Cols: 1, Attrs: Bold | Italic | Underline | Inverse}}},
		{"blink dropped", "\x1b[5ma\x1b[0mb", []Run{{Text: "ab" + "        ", Cols: 10}}},
		{"concealed", "\x1b[8mab\x1b[0mc", []Run{{Text: "  c       ", Cols: 10}}},
		{"basic", "\x1b[31ma", []Run{{Text: "a", Cols: 1, Fg: idx(1)}}},
		{"bright", "\x1b[91ma", []Run{{Text: "a", Cols: 1, Fg: idx(9)}}},
		{"extended", "\x1b[38;5;200ma", []Run{{Text: "a", Cols: 1, Fg: idx(200)}}},
		{"direct", "\x1b[38;2;10;20;30ma", []Run{{Text: "a", Cols: 1, Fg: Color{Kind: RGB, R: 10, G: 20, B: 30}}}},
		{"background", "\x1b[42ma\x1b[48;5;17mb", []Run{{Text: "a", Cols: 1, Bg: idx(2)}, {Text: "b", Cols: 1, Bg: idx(17)}}},
		{"wide", "\x1b[1m世\x1b[0mx", []Run{{Text: "世", Cols: 2, Attrs: Bold}, {Text: "x       ", Cols: 8}}},
		{"styled blanks kept", "\x1b[44m  \x1b[0m", []Run{{Text: "  ", Cols: 2, Bg: idx(4)}, {Text: "        ", Cols: 8}}},
		{"link dropped", "\x1b]8;;http://x\x1b\\a\x1b]8;;\x1b\\b", []Run{{Text: "ab        ", Cols: 10}}},
		{"wide cut at the edge", "\x1b[1;10H世", []Run{{Text: "          ", Cols: 10}}},
	} {
		got := fed(t, Size{Cols: 10, Rows: 1}, c.input).rows[0]
		if len(got) < len(c.first) || !equalRuns(got[:len(c.first)], c.first) {
			t.Errorf("%s: got %+v, want it to begin %+v", c.name, got, c.first)
		}
		if width := colsOf(got); width != 10 {
			t.Errorf("%s: the runs cover %d columns", c.name, width)
		}
	}
}

// Only the rows that changed are published again; the cursor alone moves
// the sequence; a quiet look moves nothing.
func TestOnlyChangedRowsArePublished(t *testing.T) {
	e := emulated(t, Size{Cols: 8, Rows: 3})
	var p published
	e.write([]byte("one"))
	p.take(e.look())
	if s := p.since(0); s.Seq != 1 || len(s.Changed) != 3 {
		t.Fatalf("the first look: %+v", s)
	}
	e.write([]byte("\x1b[3;1Hthree"))
	p.take(e.look())
	if s := p.since(1); s.Seq != 2 || len(s.Changed) != 1 || s.Changed[0].Index != 2 {
		t.Fatalf("one row written: %+v", s)
	}
	p.take(e.look())
	if p.seq != 2 {
		t.Fatalf("a quiet look moved the sequence to %d", p.seq)
	}
	e.write([]byte("\x1b[1;1H"))
	p.take(e.look())
	if s := p.since(2); s.Seq != 3 || len(s.Changed) != 0 || s.Cursor != (Cursor{Visible: true}) {
		t.Fatalf("the cursor moved: %+v", s)
	}
	if s := p.since(1); len(s.Changed) != 1 {
		t.Fatalf("a reader two behind gets %d rows", len(s.Changed))
	}
	e.resize(Size{Cols: 8, Rows: 4})
	p.take(e.look())
	if s := p.since(3); len(s.Changed) != 4 {
		t.Fatalf("a resize republishes %d rows", len(s.Changed))
	}
}

// The modes a key encoder needs follow what the program sets and resets,
// and a full reset clears them.
func TestTheModesFollowTheProgram(t *testing.T) {
	e := emulated(t, Size{Cols: 8, Rows: 2})
	steps := []struct {
		input string
		want  Modes
	}{
		{"\x1b[?1h\x1b[?2004h\x1b[?1002h\x1b[?1006h\x1b[?1004h\x1b[?66h", Modes{CursorKeys: true, Keypad: true, BracketedPaste: true, Mouse: MouseButton, MouseSGR: true, Focus: true}},
		{"\x1b[?1003h", Modes{CursorKeys: true, Keypad: true, BracketedPaste: true, Mouse: MouseAny, MouseSGR: true, Focus: true}},
		{"\x1b[?1002l", Modes{CursorKeys: true, Keypad: true, BracketedPaste: true, Mouse: MouseAny, MouseSGR: true, Focus: true}},
		{"\x1b[?1003l\x1b[?1l", Modes{Keypad: true, BracketedPaste: true, MouseSGR: true, Focus: true}},
		{"\x1b[?1049h", Modes{Keypad: true, BracketedPaste: true, MouseSGR: true, Focus: true, AltScreen: true}},
		{"\x1bc", Modes{}},
	}
	for _, step := range steps {
		e.write([]byte(step.input))
		if got := e.look().modes; got != step.want {
			t.Errorf("after %+q: %+v, want %+v", step.input, got, step.want)
		}
	}
	e.write([]byte("\x1b[?25l\x1b[2;4H"))
	if got := e.look().cursor; got != (Cursor{Row: 1, Col: 3}) {
		t.Errorf("a hidden, moved cursor: %+v", got)
	}
}

// OSC 52 is taken within its cap; a query, an oversized or unreadable
// request is refused and counted, and nothing of it reaches the screen.
func TestTheClipboardIsCapped(t *testing.T) {
	var taken [][]byte
	refused := 0
	e, err := newVT(Size{Cols: 8, Rows: 1}, func(p []byte) { taken = append(taken, p) }, func() { refused++ })
	if err != nil {
		t.Fatal(err)
	}
	huge := make([]byte, maxClipboard+4)
	for i := range huge {
		huge[i] = 'A'
	}
	for _, input := range []string{
		"\x1b]52;c;aGVsbG8=\x07",
		"\x1b]52;c;?\x07",
		"\x1b]52;c;not base64!\x07",
		"\x1b]52;c;" + string(huge) + "\x07",
		"\x1b]52;c;\x07",
	} {
		e.write([]byte(input))
	}
	if len(taken) != 1 || string(taken[0]) != "hello" || refused != 4 {
		t.Fatalf("taken %q, refused %d", taken, refused)
	}
	if row := e.look().rows[0]; len(row) != 1 || row[0].Text != "        " {
		t.Fatalf("the requests reached the screen: %+v", row)
	}
}
