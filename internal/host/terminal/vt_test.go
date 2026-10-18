package terminal

import "testing"

// A wide glyph is a run of its own, two columns for one glyph: how the
// workers' pane learns its width from the emulator rather than guess it.
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
