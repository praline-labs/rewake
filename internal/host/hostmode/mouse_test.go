package hostmode

import (
	"testing"

	"github.com/praline-labs/rewake/internal/host/terminal"
)

func TestMouseReportsAreReadInBothForms(t *testing.T) {
	for token, want := range map[string]mouseEvent{
		"\x1b[<0;12;24M":                      {code: 0, col: 12, row: 24},
		"\x1b[<64;1;3M":                       {code: 64, col: 1, row: 3},
		"\x1b[<2;300;7m":                      {code: 2, col: 300, row: 7, release: true},
		"\x1b[M" + string([]byte{35, 40, 57}): {code: 3, col: 8, row: 25, release: true},
	} {
		if got, ok := parseMouse([]byte(token)); !ok || got != want {
			t.Errorf("%q: %+v %v, want %+v", token, got, ok, want)
		}
	}
	for _, token := range []string{"\x1b[A", "\x1b[<0;1M", "\x1b[<a;1;2M"} {
		if _, ok := parseMouse([]byte(token)); ok {
			t.Errorf("%q read as a mouse report", token)
		}
	}
}

// Each tracking mode hears what it asked for, in the form 1006 chooses.
func TestAMouseEventReachesTheProgramAsItAsked(t *testing.T) {
	press := mouseEvent{code: 0, col: 5, row: 3}
	ctrlPress := mouseEvent{code: 16, col: 5, row: 3}
	release := mouseEvent{code: 0, col: 5, row: 3, release: true}
	drag := mouseEvent{code: 32, col: 6, row: 3}
	hover := mouseEvent{code: 35, col: 6, row: 3}
	wheel := mouseEvent{code: 64, col: 5, row: 3}
	sgr := func(mode terminal.MouseMode) terminal.Modes { return terminal.Modes{Mouse: mode, MouseSGR: true} }
	legacy := func(mode terminal.MouseMode) terminal.Modes { return terminal.Modes{Mouse: mode} }
	for _, c := range []struct {
		name  string
		event mouseEvent
		modes terminal.Modes
		want  string
	}{
		{"off", press, sgr(terminal.MouseOff), ""},
		{"x10 press", ctrlPress, legacy(terminal.MouseX10), "\x1b[M" + string([]byte{32, 37, 35})},
		{"x10 release", release, legacy(terminal.MouseX10), ""},
		{"normal release", release, legacy(terminal.MouseNormal), "\x1b[M" + string([]byte{35, 37, 35})},
		{"normal drag", drag, sgr(terminal.MouseNormal), ""},
		{"button drag", drag, sgr(terminal.MouseButton), "\x1b[<32;6;3M"},
		{"button hover", hover, sgr(terminal.MouseButton), ""},
		{"any hover", hover, sgr(terminal.MouseAny), "\x1b[<35;6;3M"},
		{"sgr release", release, sgr(terminal.MouseNormal), "\x1b[<0;5;3m"},
		{"sgr wheel", wheel, sgr(terminal.MouseNormal), "\x1b[<64;5;3M"},
		{"legacy past 223", mouseEvent{code: 0, col: 300, row: 3}, legacy(terminal.MouseNormal), ""},
	} {
		if got := string(c.event.encode(c.modes, c.event.col, c.event.row)); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// host is a host of an 80×10 terminal showing main and one worker, whose
// writes are kept rather than done.
func host(modes terminal.Modes) (*Host, *[]string) {
	var sent []string
	tabs := make([]*tab, 2)
	for i, name := range []string{"main", "w1"} {
		tabs[i] = &tab{name: name, ops: make(chan func(), 16), keys: func(p []byte) error {
			sent = append(sent, name+":"+string(p))
			return nil
		}}
		tabs[i].pic.Modes = modes
	}
	return &Host{lay: layoutFor(terminal.Size{Cols: 80, Rows: 10}, false), tabs: tabs, cur: tabs[0], canvas: terminal.NewCanvas(nopWriter{}, nil, terminal.Size{Cols: 80, Rows: 10})}, &sent
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }

func drain(h *Host) {
	for _, t := range h.tabs {
		for len(t.ops) > 0 {
			(<-t.ops)()
		}
	}
}

func TestAClickOnTheBarShowsItsTabAndNeverReachesTheProgram(t *testing.T) {
	h, sent := host(terminal.Modes{Mouse: terminal.MouseAny, MouseSGR: true})
	// The bar reads " 0 main  1 w1 ": the second label covers columns 9-14.
	h.dispatch(h.in.feed([]byte("\x1b[<0;10;10M\x1b[<0;10;10m"), h.in.heldSince))
	drain(h)
	if h.cur != h.tabs[1] || len(*sent) != 0 {
		t.Fatalf("shown %s, sent %q", h.cur.name, *sent)
	}
	h.dispatch(h.in.feed([]byte("\x1b[<0;3;2M"), h.in.heldSince))
	drain(h)
	if len(*sent) != 1 || (*sent)[0] != "w1:\x1b[<0;3;2M" {
		t.Fatalf("a click in the region sent %q", *sent)
	}
}

func TestAMouseEventIsDroppedForAProgramWithoutMouse(t *testing.T) {
	h, sent := host(terminal.Modes{})
	h.dispatch(h.in.feed([]byte("\x1b[<64;3;2M"), h.in.heldSince))
	drain(h)
	if len(*sent) != 0 {
		t.Fatalf("sent %q", *sent)
	}
}

func TestAPasteIsFramedOnlyForAProgramThatAskedForBrackets(t *testing.T) {
	for _, c := range []struct {
		modes terminal.Modes
		want  string
	}{
		{terminal.Modes{BracketedPaste: true}, "main:\x1b[200~|main:a\x1cb|main:\x1b[201~"},
		{terminal.Modes{}, "main:a\x1cb"},
	} {
		h, sent := host(c.modes)
		h.dispatch(h.in.feed([]byte("\x1b[200~a\x1cb\x1b[201~"), h.in.heldSince))
		drain(h)
		got := ""
		for i, s := range *sent {
			if i > 0 {
				got += "|"
			}
			got += s
		}
		if got != c.want {
			t.Errorf("modes %+v: sent %q, want %q", c.modes, got, c.want)
		}
	}
}
