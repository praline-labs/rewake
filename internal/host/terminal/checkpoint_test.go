package terminal

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
)

// state is everything of an emulator that changes what its later output
// does or shows. A hidden alternate screen's cells and cursor are left out:
// entering it clears the one and copies the main cursor into the other.
type state struct {
	Alt, Phantom bool
	Rows         [2][][]Run
	Cursor       [2]vt.Cursor
	Saved        [2]vt.Cursor
	Scroll       [2]uv.Rectangle
	Modes        Modes
	Title        string
	Kitty        []uint8
}

func stateOf(v *vtEmulator) state {
	c := v.checkpoint()
	s := state{Alt: c.alt, Phantom: c.phantom, Modes: v.modes, Title: v.title, Kitty: v.kitty}
	for i, screen := range c.screens {
		s.Saved[i], s.Scroll[i] = screen.saved, screen.scroll
		if i == 1 && !c.alt {
			continue
		}
		s.Cursor[i] = screen.cursor
		size := v.size()
		for y := range size.Rows {
			s.Rows[i] = append(s.Rows[i], rowOf(screen.screen.CellAt, y, size.Cols))
		}
	}
	return s
}

// differs names the fields of state that differ, with both values.
func differs(want, got state) string {
	var out []string
	w, g := reflect.ValueOf(want), reflect.ValueOf(got)
	for i := range w.NumField() {
		if !reflect.DeepEqual(w.Field(i).Interface(), g.Field(i).Interface()) {
			out = append(out, fmt.Sprintf("%s %+v, want %+v", w.Type().Field(i).Name, g.Field(i), w.Field(i)))
		}
	}
	return strings.Join(out, "; ")
}

// gridState is the host's state, read under its lock.
func gridState(g *Grid) state {
	g.mu.Lock()
	defer g.mu.Unlock()
	return stateOf(g.emu.(*vtEmulator))
}

// restored is a fresh emulator of the grid's size fed the preamble of a
// stream attached now, which a client and the next image of a host both
// start from; the stream gives what follows.
func restored(t *testing.T, g *Grid, size Size) (*vtEmulator, *Stream) {
	t.Helper()
	s := g.Attach()
	t.Cleanup(s.Detach)
	e, err := newVT(size)
	if err != nil {
		t.Fatal(err)
	}
	v := e.(*vtEmulator)
	t.Cleanup(func() { _ = v.stopReplies() })
	v.write(s.Preamble)
	return v, s
}

// checkpointed is a program with every state the checkpoint carries: the
// main screen under each kind of alternate one, scroll regions, saved
// cursors on both screens, colors and cursor styles, DECOM, DECLRMM margins,
// LNM, a pending wrap, autowrap off, and the input modes.
var checkpointed = strings.Join([]string{
	"\x1b]2;tïtle\x07\x1b[1;32mtop\x1b[0m\r\nline two\x1b[2;4r\x1b[4;1Hfoot\nscrolled",
	"\x1b[3;7H\x1b[4;33m\x1b[5 q\x1b7\x1b[0m\x1b[1;1H",
	"\x1b[?1049h\x1b[2;2HALT\x1b[1;3r\x1b[2;9H\x1b7\x1b[?25l\x1b[?1049l",
	"\x1b[?1047h\x1b[3;3Hbare\x1b[?1047l\x1b[?47h\x1b[2;2Hold\x1b[?47l",
	"\x1b[?6h\x1b[1;1HO\x1b[?6l",
	"\x1b[?69h\x1b[3;10s\x1b[2;3HMARGINSWRAPPED\x1b[?69l",
	"\x1b[20hA\nB\x1b[20l",
	"\x1b[r\x1b[5;1H" + strings.Repeat("X", 20),
	"\x1b[?7l\x1b[4;15H" + strings.Repeat("Z", 8) + "\x1b[?7h",
	"\x1b[?1h\x1b[?2004h\x1b[>4;2m\x1b[=1;1u\x1b[>3u",
	"\x1b[?1049h\x1b[2;4r\x1b[?6h\x1b[2;2H\x1b[7mshown\x1b7\x1b[1 q",
}, "")

// At every cut of the program, inside a sequence or a character too, a
// fresh emulator fed the preamble holds the host's state, and both hold the
// same again after the rest of the program.
func TestThePreambleCarriesTheStateLaterOutputUses(t *testing.T) {
	size := Size{Cols: 20, Rows: 5}
	for cut := 0; cut <= len(checkpointed); cut++ {
		g := newTestGrid(t)
		g.commit([]byte(checkpointed[:cut]), false)
		client, s := restored(t, g, size)
		if diff := differs(gridState(g), stateOf(client)); diff != "" {
			t.Fatalf("cut at %d: %s", cut, diff)
		}
		g.commit([]byte(checkpointed[cut:]), false)
		client.write(drained(s))
		if diff := differs(gridState(g), stateOf(client)); diff != "" {
			t.Fatalf("cut at %d, after the rest: %s", cut, diff)
		}
	}
}

// Each piece of state, with the output that would go wrong without it: the
// reviewer's five counterexamples first (November 2, 2026), then the rest of
// the checkpoint. The host and the restored emulator end the same.
func TestTheContinuationsTheCheckpointChanges(t *testing.T) {
	for _, c := range []struct{ name, before, after string }{
		{"main screen under alternate", "normal\x1b[?1049hALT", "\x1b[?1049l"},
		{"scroll region", "\x1b[1;1HAAA\x1b[2;1HBBB\x1b[3;1HCCC\x1b[4;1HDDD\x1b[2;3r\x1b[3;1H", "\nX"},
		{"saved cursor", "a\x1b7\x1b[3;1Hb", "\x1b8X"},
		{"pending wrap", strings.Repeat("X", 20), "Y"},
		{"autowrap disabled", "\x1b[?7l" + strings.Repeat("X", 20), "YZ"},
		{"main screen under 1047", "normal\x1b[?1047hALT", "\x1b[?1047lX"},
		{"saved cursor of the hidden alternate", "\x1b[?1047h\x1b[3;5H\x1b7\x1b[?1047l", "\x1b[?1047h\x1b8Q"},
		{"saved cursor's pen and style", "\x1b[2;2H\x1b[1;35m\x1b[4 q\x1b7\x1b[0m\x1b[5;5H", "\x1b8P"},
		{"origin mode", "\x1b[2;4r\x1b[?6h\x1b[2;1HO", "\x1b[1;1HQ\x1b[9;1HR"},
		{"left and right margins", "\x1b[?69h\x1b[3;8s\x1b[1;3H", "LONGERTHANTHEMARGINS"},
		{"newline mode", "\x1b[20h\x1b[2;5H", "A\nB"},
		{"a scroll region on the alternate screen", "\x1b[?1049h\x1b[3;4r\x1b[4;1H", "\nX\nY"},
		{"pending wrap in a color", "\x1b[41m" + strings.Repeat("X", 19) + "\x1b[42mW\x1b[43m", "Y"},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := newTestGrid(t)
			g.commit([]byte(c.before), false)
			client, s := restored(t, g, Size{Cols: 20, Rows: 5})
			g.commit([]byte(c.after), false)
			client.write(drained(s))
			if diff := differs(gridState(g), stateOf(client)); diff != "" {
				t.Fatalf("after %q the restored emulator differs: %s", c.after, diff)
			}
		})
	}
}

// A resize while the alternate screen is shown resizes both screens and
// resets their margins; the main screen beneath comes back at the new size.
func TestTheCheckpointAfterAResizeUnderTheAlternateScreen(t *testing.T) {
	g := newTestGrid(t)
	g.commit([]byte("normal\x1b[2;4r\x1b[?1049h\x1b[2;3rALT"), false)
	size := Size{Cols: 30, Rows: 8}
	if err := g.Resize(size); err != nil {
		t.Fatal(err)
	}
	client, s := restored(t, g, size)
	if diff := differs(gridState(g), stateOf(client)); diff != "" {
		t.Fatalf("restored: %s", diff)
	}
	g.commit([]byte("\x1b[8;1H\nwide enough for thirty\x1b[?1049lX"), false)
	client.write(drained(s))
	if diff := differs(gridState(g), stateOf(client)); diff != "" {
		t.Fatalf("after the rest: %s", diff)
	}
}

// A grid seeded with a closed grid's seed holds its state, gives none of the
// answers the seed's writing makes (?2048h reports the size), and answers
// the program's next query, here the end of one the old grid parsed half of:
// the next image's grid after a handover.
func TestASeededGridHoldsTheStateHandedOver(t *testing.T) {
	g := newTestGrid(t)
	g.commit([]byte(checkpointed+"\x1b[?2048h\x1b[6"), false)
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := NewSeededGrid(g.Size(), g.Seed())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = next.Close() })
	if diff := differs(gridState(g), gridState(next)); diff != "" {
		t.Fatalf("the seeded grid differs: %s", diff)
	}
	next.Write([]byte("n"))
	select {
	case answer := <-next.Answers():
		if !strings.HasSuffix(string(answer), "R") {
			t.Fatalf("the seeded grid answered %q first, want the cursor's report", answer)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the seeded grid did not answer the program's query")
	}
}

// The library is the version whose unexported fields the checkpoint reads.
func TestTheTerminalLibraryKeepsTheFieldsTheCheckpointReads(t *testing.T) {
	if err := checkLibrary(); err != nil {
		t.Fatal(err)
	}
}
