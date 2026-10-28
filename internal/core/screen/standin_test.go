package screen

import (
	"strings"
	"testing"
)

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
