package hostmode

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/host/terminal"
)

// splitHost is host on a terminal of 121×10, split: main in columns 1-60,
// the divider in 61, w1 in 62-121.
func splitHost(modes terminal.Modes) (*Host, *[]string) {
	h, sent := host(modes)
	size := terminal.Size{Cols: 121, Rows: 10}
	h.canvas.Resize(size)
	h.relayout(size)
	drain(h)
	return h, sent
}

func feed(h *Host, keys string) {
	h.dispatch(h.in.feed([]byte(keys), h.in.heldSince))
	drain(h)
}

// A mouse event goes to the program under the pointer at its own cell,
// whichever has the focus; a left click focuses it, the wheel does not.
func TestAMouseEventGoesToThePaneUnderIt(t *testing.T) {
	h, sent := splitHost(terminal.Modes{Mouse: terminal.MouseAny, MouseSGR: true})
	feed(h, "\x1b[<64;70;2M")
	if strings.Join(*sent, "|") != "w1:\x1b[<64;9;2M" || h.focus != h.tabs[0] {
		t.Fatalf("wheel over w1: sent %q, focus %s", *sent, h.focus.name)
	}
	*sent = nil
	feed(h, "\x1b[<0;62;3M\x1b[<0;62;3m\x1b[<0;61;3M")
	if strings.Join(*sent, "|") != "w1:\x1b[<0;1;3M|w1:\x1b[<0;1;3m" || h.focus != h.tabs[1] {
		t.Fatalf("click on w1, then the divider: sent %q, focus %s", *sent, h.focus.name)
	}
	feed(h, "abc")
	if (*sent)[len(*sent)-1] != "w1:abc" {
		t.Fatalf("keys after the click: %q", *sent)
	}
}

// A drag stays with the program it began in, held to its pane's edge, and
// the release reaches it: a selection across the divider ends where it began.
func TestADragStaysWithItsProgram(t *testing.T) {
	h, sent := splitHost(terminal.Modes{Mouse: terminal.MouseButton, MouseSGR: true})
	feed(h, "\x1b[<0;50;4M\x1b[<32;90;4M\x1b[<0;90;4m\x1b[<35;90;4M")
	want := "main:\x1b[<0;50;4M|main:\x1b[<32;60;4M|main:\x1b[<0;60;4m"
	if strings.Join(*sent, "|") != want {
		t.Fatalf("sent %q, want %q", *sent, want)
	}
}

// The prefix's arrows move the focus between the halves, telling a program
// that asked for focus reports; z gives the focused program the whole
// terminal and back; the keys follow the focus.
func TestTheArrowsMoveTheFocusAndZZooms(t *testing.T) {
	h, sent := splitHost(terminal.Modes{Focus: true})
	feed(h, "\x1c\x1b[Ca")
	if h.focus != h.tabs[1] || strings.Join(*sent, "|") != "main:\x1b[O|w1:\x1b[I|w1:a" {
		t.Fatalf("right: focus %s, sent %q", h.focus.name, *sent)
	}
	feed(h, "\x1cz")
	if h.lay.split || h.lay.work.Cols != 121 || h.tabs[1].at.Cols != 121 || h.tabs[0].at.Cols != 121 {
		t.Fatalf("zoomed: %+v, sizes %+v %+v", h.lay, h.tabs[0].at, h.tabs[1].at)
	}
	if p := h.panes(); len(p) != 1 || p[0].tab != h.tabs[1] {
		t.Fatalf("zoomed panes %+v", p)
	}
	feed(h, "\x1cz\x1c\x1bOD")
	if !h.lay.split || h.tabs[0].at.Cols != 60 || h.focus != h.tabs[0] {
		t.Fatalf("back: %+v, focus %s", h.lay, h.focus.name)
	}
}
