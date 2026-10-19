package terminal

import (
	"context"
	"testing"
	"time"
)

// What the host passes on is taken from the program's bytes whole: a title
// with its non-ASCII characters, a copy, a notification, a bell; a copy's
// query and a progress report are not events. Cut at every offset, as reads
// come. Within one write the OSC strings come before a bell, being taken
// before the emulator sees the bytes; their order among themselves holds.
func TestTitleAndEventsComeWhole(t *testing.T) {
	stream := "\x1b]0;✳ Claude Code\x07\x1b]52;c;aGk=\x1b\\\x1b]52;c;?\x07\x1b]9;4;1;50\x07\x1b]9;done ✓\x07\a\x1b]777;notify;a;b\x07"
	want := []Event{{OSC: "52;c;aGk="}, {OSC: "9;done ✓"}, {Bell: true}, {OSC: "777;notify;a;b"}}
	for cut := 0; cut <= len(stream); cut++ {
		emulator, err := newVT(Size{Cols: 20, Rows: 1})
		if err != nil {
			t.Fatal(err)
		}
		vt := emulator.(*vtEmulator)
		vt.write([]byte(stream[:cut]))
		vt.write([]byte(stream[cut:]))
		l := vt.look()
		_ = vt.stopReplies()
		if l.title != "✳ Claude Code" || len(l.events) != len(want) {
			t.Fatalf("cut at %d: title %q, events %+v", cut, l.title, l.events)
		}
		seen := map[Event]int{}
		var oscs []Event
		for _, e := range l.events {
			seen[e]++
			if !e.Bell {
				oscs = append(oscs, e)
			}
		}
		for _, e := range want {
			seen[e]--
		}
		for e, n := range seen {
			if n != 0 {
				t.Fatalf("cut at %d: events %+v, want %+v (%+v)", cut, l.events, want, e)
			}
		}
		if oscs[0] != want[0] || oscs[1] != want[1] || oscs[2] != want[3] {
			t.Fatalf("cut at %d: the OSC strings out of order: %+v", cut, oscs)
		}
	}
}

// A reader gets the events since its sequence, once; one that had none —
// a view opened late — gets none, so an old copy is never set again.
func TestEventsReachOnlyAReaderThatWasThere(t *testing.T) {
	g, err := NewGrid(Size{Cols: 10, Rows: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = g.Close() }()
	ctx := context.Background()
	first := g.Screen(ctx, 0)
	g.Write([]byte("x\a"))
	s := g.Screen(ctx, first.Seq)
	for len(s.Events) == 0 && s.Seq == first.Seq {
		s = g.Screen(ctx, first.Seq)
	}
	if len(s.Events) != 1 || !s.Events[0].Bell {
		t.Fatalf("a reader that was there got %+v", s.Events)
	}
	if late := g.Screen(ctx, 0); len(late.Events) != 0 {
		t.Fatalf("a late reader got %+v", late.Events)
	}
	soon, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if again := g.Screen(soon, s.Seq); len(again.Events) != 0 {
		t.Fatalf("the same events twice: %+v", again.Events)
	}
}

// A program asking for the background hears the person's terminal's, which
// the host mode seeds every emulator with, not the library's black.
func TestTheDefaultColorsAnswerTheProgram(t *testing.T) {
	g, err := NewGrid(Size{Cols: 10, Rows: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = g.Close() }()
	g.SetDefaults(ParseDefaults(FormatDefaults("rgb:d4d4/d4d4/d4d4", "rgb:fafa/f0f0/e6e6")))
	g.Write([]byte("\x1b]11;?\x07"))
	select {
	case got := <-g.Answers():
		if string(got) != "\x1b]11;rgb:fafa/f0f0/e6e6\x07" {
			t.Fatalf("answered %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("no answer")
	}
}
