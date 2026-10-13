package terminal

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateCanary = flag.Bool("canary.update", false, "rewrite testdata/canary.golden from the emulator")

// canaryStream touches what a library update is likeliest to change: a
// title that must not reach the grid, dim text, an emoji with VS16, wide
// glyphs, a scroll region scrolled, and the alternate screen entered and
// left.
const canaryStream = "\x1b]0;title\x07" +
	"\x1b[2mdim\x1b[0m ❤️ 世界\r\n" +
	"two\r\nthree\r\nfour\r\nfive" +
	"\x1b[2;4r\x1b[4;1H\n\x1b[1mnew\x1b[0m\x1b[r" +
	"\x1b[?1049halt\x1b[?1049l"

// The emulator against a known byte stream: a library update that changes a
// width, a wrap or a scroll fails here, in one small unit, before it shows
// on every screen. Rewrite the golden with -args -canary.update, and review
// the difference like code.
func TestTheEmulatorCanary(t *testing.T) {
	got := describe(fed(t, Size{Cols: 16, Rows: 5}, canaryStream))
	path := filepath.Join("testdata", "canary.golden")
	if *updateCanary {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no golden; write it with -args -canary.update and review it: %v", err)
	}
	if got != string(want) {
		t.Fatalf("the emulator draws the canary otherwise:\n--- got\n%s--- want\n%s", got, want)
	}
}

// describe is a look as plain lines: each row's text, then its runs that
// carry a style, then the cursor and the modes.
func describe(l look) string {
	var b strings.Builder
	for i, row := range l.rows {
		var text strings.Builder
		var styled []string
		col := 0
		for _, run := range row {
			text.WriteString(run.Text)
			if run.Fg != (Color{}) || run.Bg != (Color{}) || run.Attrs != 0 {
				styled = append(styled, fmt.Sprintf("%d+%d %s fg=%v bg=%v", col, run.Cols, attrNames(run.Attrs), run.Fg, run.Bg))
			}
			col += run.Cols
		}
		fmt.Fprintf(&b, "%d %q\n", i, strings.TrimRight(text.String(), " "))
		for _, s := range styled {
			fmt.Fprintf(&b, "  %s\n", s)
		}
	}
	fmt.Fprintf(&b, "cursor %+v\nmodes %+v\n", l.cursor, l.modes)
	return b.String()
}

func attrNames(a Attrs) string {
	var names []string
	for i, name := range []string{"bold", "dim", "italic", "underline", "inverse", "strikethrough"} {
		if a&(1<<i) != 0 {
			names = append(names, name)
		}
	}
	return "[" + strings.Join(names, ",") + "]"
}
