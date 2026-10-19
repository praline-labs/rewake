package hostmode

import "testing"

// The colors are read off the person's input up to the device attributes,
// what was typed meanwhile kept; an answer coming after the wait is dropped,
// never typed into a program.
func TestTheTerminalsColorsAreReadOffItsInput(t *testing.T) {
	keys := make(chan []byte, 4)
	keys <- []byte("a\x1b]10;rgb:d4d4/d4d4/d4d4\x1b\\\x1b]11;rgb:1e1e/1e")
	keys <- []byte("1e/1e1e\x07b\x1b[?62;22c")
	var asked string
	fg, bg, rest := askColors(func(p []byte) error { asked = string(p); return nil }, keys)
	if asked != colorQuery || fg != "rgb:d4d4/d4d4/d4d4" || bg != "rgb:1e1e/1e1e/1e1e" || string(rest) != "ab" {
		t.Fatalf("asked %q; fg %q, bg %q, rest %q", asked, fg, bg, rest)
	}
	if sent, _ := run("x\x1b]11;rgb:0/0/0\x1b\\\x1b[?62;22cy"); sent != "xy" {
		t.Fatalf("a late answer reached the program: %q", sent)
	}
}
