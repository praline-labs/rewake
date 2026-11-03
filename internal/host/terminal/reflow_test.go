package terminal

import (
	"slices"
	"strings"
	"testing"
)

// shownRows is the text of the rows of an emulator's screen, trailing blanks
// cut.
func shownRows(v *vtEmulator) []string {
	var out []string
	for _, row := range v.look().rows {
		var text strings.Builder
		for _, run := range row {
			text.WriteString(run.Text)
		}
		out = append(out, strings.TrimRight(text.String(), " "))
	}
	return out
}

// A line main wrote across a wide window and a pane then narrowed: the
// host's grid wraps it as a terminal does, and a client attaching after is
// drawn both parts, the cursor after the text. The grid used to cut the row,
// the loss of the third line of main's intro after a shell restart.
func TestANarrowedGridWrapsWhatARowHeldAndTheReplayCarriesIt(t *testing.T) {
	g, err := NewGrid(Size{Cols: 40, Rows: 5})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = g.Close() })
	g.commit([]byte("one\r\nrewake: role main: selected explicitly\r\ntail"), false)
	if err := g.Resize(Size{Cols: 20, Rows: 5}); err != nil {
		t.Fatal(err)
	}
	s := g.Attach()
	client := freshVT(t)
	client.write(s.Preamble)
	want := []string{"one", "rewake: role main: s", "elected explicitly", "tail", ""}
	if got := shownRows(client); !slices.Equal(got, want) {
		t.Fatalf("rows after the replay %q, want %q", got, want)
	}
	if cursor := client.look().cursor; cursor.Row != 3 || cursor.Col != 4 {
		t.Fatalf("cursor at %d,%d, want 3,4", cursor.Row, cursor.Col)
	}
}

// A grid narrowed twice joins what the first wrap broke before it wraps
// again: the worker pane's intro showed a break in the middle of its line.
func TestAGridNarrowedTwiceWrapsTheLineOnce(t *testing.T) {
	g, err := NewGrid(Size{Cols: 60, Rows: 5})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = g.Close() })
	g.commit([]byte("0123456789abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ-end"), false)
	for _, cols := range []int{40, 30} {
		if err := g.Resize(Size{Cols: cols, Rows: 5}); err != nil {
			t.Fatal(err)
		}
	}
	g.mu.Lock()
	rows := shownRows(g.emu.(*vtEmulator))
	g.mu.Unlock()
	want := []string{"0123456789abcdefghijklmnopqrst", "uvwxyz0123456789ABCDEFGHIJKLMN", "OPQRSTUVWXYZ-end", "", ""}
	if !slices.Equal(rows, want) {
		t.Fatalf("rows %q, want %q", rows, want)
	}
}

// The rows that no longer fit leave from the top, so the cursor stays on the
// screen.
func TestANarrowedGridKeepsTheCursorOnTheScreen(t *testing.T) {
	g, err := NewGrid(Size{Cols: 40, Rows: 4})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = g.Close() })
	g.commit([]byte("aaaaaaaaaabbbbbbbbbbccccccccccdd\r\nshort\r\nxxxxxxxxxxyyyyyyyyyyzzzzzzzzzzww\r\n$ "), false)
	if err := g.Resize(Size{Cols: 20, Rows: 4}); err != nil {
		t.Fatal(err)
	}
	g.mu.Lock()
	host := g.emu.(*vtEmulator)
	rows, cursor := shownRows(host), host.look().cursor
	g.mu.Unlock()
	want := []string{"short", "xxxxxxxxxxyyyyyyyyyy", "zzzzzzzzzzww", "$"}
	if !slices.Equal(rows, want) {
		t.Fatalf("rows %q, want %q", rows, want)
	}
	if cursor.Row != 3 || cursor.Col != 2 {
		t.Fatalf("cursor at %d,%d, want 3,2", cursor.Row, cursor.Col)
	}
}
