package terminal

import (
	"testing"
	"time"
)

// A program asking for the background hears the person's terminal's, which
// the shell seeds every space host with, not the library's black.
func TestTheDefaultColorsAnswerTheProgram(t *testing.T) {
	g, err := NewGrid(Size{Cols: 10, Rows: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = g.Close() }()
	g.SetDefaults(ParseDefaults("rgb:d4d4/d4d4/d4d4;rgb:fafa/f0f0/e6e6"))
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
