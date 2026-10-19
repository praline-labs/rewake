package terminal

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

// A program's output with every mode the preamble carries, a kitty stack of
// three, a title, the alternate screen, colors and a string, so a cut lands
// inside each kind of sequence and inside a character.
const program = "top\r\n\x1b[?1h\x1b[?66h\x1b[?2004h\x1b[?1004h\x1b[?1002h\x1b[?1006h\x1b[>4;2m" +
	"\x1b[=1;1u\x1b[>3u\x1b[>11u\x1b]2;tïtle\x07\x1b[?1049h\x1b[2;3H\x1b[1;31mré\x1b[38;5;200mX" +
	"\x1b[48;2;1;2;3m界\x1b[0m\x1bP+q544e\x1b\\\x1b[4;1Hend"

func newTestGrid(t *testing.T) *Grid {
	t.Helper()
	g, err := NewGrid(Size{Cols: 20, Rows: 5})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = g.Close() })
	return g
}

func freshVT(t *testing.T) *vtEmulator {
	t.Helper()
	e, err := newVT(Size{Cols: 20, Rows: 5})
	if err != nil {
		t.Fatal(err)
	}
	v := e.(*vtEmulator)
	t.Cleanup(func() { _ = v.stopReplies() })
	return v
}

// drained is what a stream's Live holds now, joined.
func drained(s *Stream) []byte {
	var out []byte
	for {
		select {
		case p, ok := <-s.Live:
			if !ok {
				return out
			}
			out = append(out, p...)
		default:
			return out
		}
	}
}

// A client attached at any cut — inside a CSI, an OSC, a DCS, a UTF-8
// character — and fed the preamble and then the live bytes stands where the
// host's emulator stands: the same cells, cursor, title, modes and kitty
// stack. The live bytes are exactly the program's after the cut, none lost
// or repeated.
func TestAClientAttachedAtAnyCutStandsWhereTheHostDoes(t *testing.T) {
	for cut := 0; cut <= len(program); cut++ {
		g := newTestGrid(t)
		g.commit([]byte(program[:cut]), false)
		s := g.Attach()
		g.commit([]byte(program[cut:]), false)
		live := drained(s)
		if !bytes.Equal(live, []byte(program[cut:])) {
			t.Fatalf("cut at %d: live %q, want %q", cut, live, program[cut:])
		}
		client := freshVT(t)
		client.write(s.Preamble)
		client.write(live)
		host := g.emu.(*vtEmulator)
		g.mu.Lock()
		want, got := host.look(), client.look()
		hostKitty := slices.Clone(host.kitty)
		g.mu.Unlock()
		if !slices.EqualFunc(want.rows, got.rows, slices.Equal) {
			t.Fatalf("cut at %d: rows %+v, want %+v", cut, got.rows, want.rows)
		}
		if got.cursor != want.cursor || got.modes != want.modes || got.title != want.title {
			t.Fatalf("cut at %d: cursor %+v modes %+v title %q, want %+v %+v %q",
				cut, got.cursor, got.modes, got.title, want.cursor, want.modes, want.title)
		}
		if !slices.Equal(client.kitty, hostKitty) {
			t.Fatalf("cut at %d: kitty stack %v, want %v", cut, client.kitty, hostKitty)
		}
		s.Detach()
	}
}

// The case found against the widget: after ESC [ ? 2004 the host's emulator
// shows paste still off, and a client that missed the bytes took the h for
// text and sent a paste bare.
func TestThePreambleEndsWithTheUnfinishedSequence(t *testing.T) {
	g := newTestGrid(t)
	g.commit([]byte("\x1b[?2004"), false)
	s := g.Attach()
	if !strings.HasSuffix(string(s.Preamble), "\x1b[?2004") {
		t.Fatalf("preamble ends %q, want the unfinished \\x1b[?2004", s.Preamble[max(0, len(s.Preamble)-12):])
	}
	g.commit([]byte("h"), false)
	client := freshVT(t)
	client.write(s.Preamble)
	client.write(drained(s))
	if !client.modes.BracketedPaste {
		t.Fatal("the client kept bracketed paste off")
	}
}

// Every mode a key's bytes depend on is in the preamble, the kitty stack as
// one push per entry in order, so the program's later pop finds what it
// pushed before.
func TestThePreambleCarriesEveryInputMode(t *testing.T) {
	g := newTestGrid(t)
	g.commit([]byte(program), false)
	s := g.Attach()
	for _, mode := range []string{
		"\x1b[?1049h", "\x1b[?1h", "\x1b[?66h", "\x1b[?2004h", "\x1b[?1004h",
		"\x1b[?1002h", "\x1b[?1006h", "\x1b[>4;2m", "\x1b[=1;1u\x1b[>3u\x1b[>11u", "\x1b]2;tïtle\x07",
	} {
		if !strings.Contains(string(s.Preamble), mode) {
			t.Errorf("preamble lacks %q", mode)
		}
	}
	g.commit([]byte("\x1b[<u"), false)
	client := freshVT(t)
	client.write(s.Preamble)
	client.write(drained(s))
	if client.modes.KittyKeys != 3 {
		t.Fatalf("after a pop the client's kitty flags are %d, want 3", client.modes.KittyKeys)
	}
}

// The end of the output closes every stream, and one attached after it
// gets the preamble and a closed stream.
func TestTheEndClosesTheStreams(t *testing.T) {
	g := newTestGrid(t)
	s := g.Attach()
	g.commit([]byte("bye"), true)
	if string(drained(s)) != "bye" {
		t.Fatal("the last piece did not reach the stream")
	}
	if _, ok := <-s.Live; ok {
		t.Fatal("the stream stayed open after the end")
	}
	late := g.Attach()
	if _, ok := <-late.Live; ok {
		t.Fatal("a stream attached after the end is open")
	}
	if !strings.Contains(string(late.Preamble), "bye") {
		t.Fatal("a late preamble lacks the screen")
	}
}
