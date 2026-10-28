package screen

import (
	"strings"
	"testing"
)

// A producer answers only a frame inside the bounds and under MaxAnswer.
func TestMarshalRefusesWhatTheCLIWouldNotRead(t *testing.T) {
	if _, err := (&Frame{Cols: MaxCols + 1, Rows: 1, Lines: [][]Run{nil}}).Marshal(); err == nil {
		t.Fatal("a window over 400 columns was marshaled")
	}
	big := &Frame{Cols: MaxCols, Rows: MaxRows}
	for y := 0; y < MaxRows; y++ {
		line := make([]Run, MaxCols)
		for x := range line {
			line[x] = Run{T: strings.Repeat("x", 8), W: 1, Fg: "#112233", Bg: "#445566", A: Bold}
		}
		big.Lines = append(big.Lines, line)
	}
	if _, err := big.Marshal(); err == nil || !strings.Contains(err.Error(), "as an answer") {
		t.Fatalf("a window over 4 MiB: %v", err)
	}
	if data, err := sample().Marshal(); err != nil || len(data) == 0 {
		t.Fatalf("an ordinary window: %v", err)
	}
}

func TestStandInNamesWhatTheTerminalDidNotReport(t *testing.T) {
	palette := make([]string, 16)
	for i := range palette {
		palette[i] = "#101010"
	}
	for name, tc := range map[string]struct {
		f    Frame
		want string
	}{
		"nothing":  {Frame{}, "foreground,background,palette"},
		"all":      {Frame{Fg: "#ffffff", Bg: "#000000", Palette: palette}, ""},
		"one gone": {Frame{Fg: "#ffffff", Bg: "#000000", Palette: append([]string{""}, palette[1:]...)}, "palette"},
		"colors":   {Frame{Fg: "#ffffff", Bg: "#000000"}, "palette"},
	} {
		if got := strings.Join(tc.f.StandIn(), ","); got != tc.want {
			t.Errorf("%s: StandIn = %q, want %q", name, got, tc.want)
		}
	}
}
