package terminal

import (
	"bytes"
	"strings"
	"testing"
)

// The person's palette paints a program's indexed colors (the owner's wish,
// docs/v2/host-mode.md): an index reaches the terminal as an index, on a
// 256-color terminal and a true-color one alike. TTY_FORCE has the
// renderer take the buffer for a terminal.
func TestAnIndexedColorReachesTheTerminalAsAnIndex(t *testing.T) {
	for _, env := range [][]string{{"TTY_FORCE=1", "TERM=xterm-256color"}, {"TTY_FORCE=1", "TERM=xterm-256color", "COLORTERM=truecolor"}} {
		var out bytes.Buffer
		c := NewCanvas(&out, env, Size{Cols: 20, Rows: 3})
		if err := c.Enter(); err != nil {
			t.Fatal(err)
		}
		err := c.Draw(Frame{
			Panes: []Pane{{Size: Size{Cols: 20, Rows: 2}, Rows: [][]Run{{
				{Text: "a", Cols: 1, Fg: Color{Kind: Indexed, Index: 1}},
				{Text: "b", Cols: 1, Fg: Color{Kind: Indexed, Index: 123}},
				{Text: "c", Cols: 1, Bg: Color{Kind: Indexed, Index: 12}},
			}}}},
			Cursor: Cursor{Row: 0, Col: 3, Visible: true},
			// A program with no mouse and no paste of its own: the host's
			// are on all the same.
			Modes: Modes{CursorKeys: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		got := out.String()
		for _, want := range []string{"31m", "38;5;123m", "104m", "\x1b[?1h", "\x1b[?2004h", "\x1b[?1002h", "\x1b[?1006h"} {
			if !strings.Contains(got, want) {
				t.Errorf("%v: %q lacks %q", env, got, want)
			}
		}
		if strings.Contains(got, "38;2;") || strings.Contains(got, "48;2;") {
			t.Errorf("%v: an index became RGB: %q", env, got)
		}
		out.Reset()
		if err := c.Leave(); err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"\x1b[?1l", "\x1b[?2004l", "\x1b[?1002l", "\x1b[?1006l", "\x1b[?1049l", "\x1b[?25h"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("%v: leaving, %q lacks %q", env, out.String(), want)
			}
		}
	}
}

// The shown program's keyboard protocols reach the person's terminal: its
// kitty flags as one entry pushed over the terminal's own, replaced at a
// change, popped at the leave only when pushed, so the person's shell keeps
// its own entry; modifyOtherKeys set and reset.
func TestTheKeyboardProtocolsAreMirrored(t *testing.T) {
	var out bytes.Buffer
	c := NewCanvas(&out, []string{"TTY_FORCE=1", "TERM=xterm-256color"}, Size{Cols: 20, Rows: 3})
	if err := c.Enter(); err != nil {
		t.Fatal(err)
	}
	draw := func(m Modes) string {
		out.Reset()
		if err := c.Draw(Frame{Modes: m}); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	if got := draw(Modes{KittyKeys: 5, ModifyOtherKeys: 2}); !strings.Contains(got, "\x1b[>5u") || !strings.Contains(got, "\x1b[>4;2m") || strings.Contains(got, "\x1b[<") {
		t.Errorf("first: %q", got)
	}
	if got := draw(Modes{KittyKeys: 7}); !strings.Contains(got, "\x1b[<1u\x1b[>7u") || !strings.Contains(got, "\x1b[>4m") {
		t.Errorf("switch: %q", got)
	}
	out.Reset()
	if err := c.Leave(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); strings.Count(got, "\x1b[<1u") != 1 {
		t.Errorf("leaving with flags pushed: %q", got)
	}
	_ = draw(Modes{})
	out.Reset()
	if err := c.Leave(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); strings.Contains(got, "\x1b[<") {
		t.Errorf("leaving with nothing pushed popped: %q", got)
	}
}

// The shown program's title is set on the person's terminal, whose own is
// saved on entering and given back on leaving; an event goes out as it came.
func TestTheTitleAndEventsReachTheTerminal(t *testing.T) {
	var out bytes.Buffer
	c := NewCanvas(&out, []string{"TTY_FORCE=1", "TERM=xterm-256color"}, Size{Cols: 20, Rows: 3})
	if err := c.Enter(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), pushTitle) {
		t.Fatalf("entering: %q", out.String())
	}
	out.Reset()
	if err := c.Draw(Frame{Title: "✳ a"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Event(Event{OSC: "52;c;aGk="}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "\x1b]2;✳ a\a") || !strings.HasSuffix(got, "\x1b]52;c;aGk=\a") {
		t.Fatalf("drawn %q", got)
	}
	out.Reset()
	if err := c.Leave(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), popTitle) {
		t.Fatalf("leaving: %q", out.String())
	}
}

// A wide glyph's run is one cell over its columns; any other run a cell per
// cluster, padded to its width.
func TestRunsBecomeCellsByTheEmulatorsWidths(t *testing.T) {
	wide := cellsOf(Run{Text: "漢", Cols: 2})
	if len(wide) != 1 || wide[0].Width != 2 {
		t.Fatalf("wide: %+v", wide)
	}
	plain := cellsOf(Run{Text: "éx", Cols: 3})
	if len(plain) != 3 || plain[0].Content != "é" || plain[1].Content != "x" || plain[2].Content != " " {
		t.Fatalf("plain: %+v", plain)
	}
}
