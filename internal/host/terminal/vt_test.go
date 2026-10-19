package terminal

import "testing"

// A wide glyph is a run of its own, two columns for one glyph: how the
// preamble places the run after it by the emulator's width.
func TestAWideGlyphIsARunOfItsOwn(t *testing.T) {
	emulator, err := newVT(Size{Cols: 8, Rows: 1})
	if err != nil {
		t.Fatal(err)
	}
	vt := emulator.(*vtEmulator)
	defer func() { _ = vt.stopReplies() }()
	vt.write([]byte("✅x界ab"))
	runs := vt.row(0, 8)
	want := []Run{{Text: "✅", Cols: 2}, {Text: "x", Cols: 1}, {Text: "界", Cols: 2}, {Text: "ab ", Cols: 3}}
	if len(runs) != len(want) {
		t.Fatalf("runs %+v, want %+v", runs, want)
	}
	for i := range want {
		if runs[i].Text != want[i].Text || runs[i].Cols != want[i].Cols {
			t.Fatalf("run %d is %+v, want %+v; all %+v", i, runs[i], want[i], runs)
		}
	}
}

// A title with a character whose UTF-8 holds the byte 0x9C stays a title,
// kept whole for a late client's preamble: the library took that byte for
// the end of the string and drew the rest of Claude Code's title over its
// logo. Cut at every offset, as reads come.
func TestATitleIsNeverDrawn(t *testing.T) {
	stream := "\x1b]0;✳ Claude Code\x07X\x1b]2;✳ é\x1b\\Y"
	for cut := 0; cut <= len(stream); cut++ {
		emulator, err := newVT(Size{Cols: 20, Rows: 1})
		if err != nil {
			t.Fatal(err)
		}
		vt := emulator.(*vtEmulator)
		vt.write([]byte(stream[:cut]))
		vt.write([]byte(stream[cut:]))
		runs := vt.row(0, 20)
		_ = vt.stopReplies()
		if len(runs) == 0 || runs[0].Text[:2] != "XY" || vt.title != "✳ é" {
			t.Fatalf("cut at %d: row %+v, title %q, want XY and ✳ é", cut, runs, vt.title)
		}
	}
}

// DECRQM 2026 is answered as a known mode, reset and then set inside a
// frame, so a program that asks before it draws wraps its frames in
// synchronized output, which the widget of a host-fed pane batches.
func TestSynchronizedOutputIsAnsweredAsKnown(t *testing.T) {
	emulator, err := newVT(Size{Cols: 8, Rows: 1})
	if err != nil {
		t.Fatal(err)
	}
	vt := emulator.(*vtEmulator)
	defer func() { _ = vt.stopReplies() }()
	ask := func(want string) {
		t.Helper()
		// The library's reply pipe is synchronous: the write waits for a reader.
		got := make(chan string)
		go func() {
			buf := make([]byte, 64)
			n, _ := vt.replies().Read(buf)
			got <- string(buf[:n])
		}()
		vt.write([]byte("\x1b[?2026$p"))
		if g := <-got; g != want {
			t.Fatalf("answer %q, want %q", g, want)
		}
	}
	ask("\x1b[?2026;2$y")
	vt.write([]byte("\x1b[?2026h"))
	ask("\x1b[?2026;1$y")
}
