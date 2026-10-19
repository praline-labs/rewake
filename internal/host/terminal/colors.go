package terminal

import (
	"image/color"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// DefaultsEnv carries the person's terminal's default colors from the host
// mode to the space hosts its main opens, so a worker's emulator answers
// them from its start (docs/v2/host-mode.md#scrollback-selection-copy): the
// foreground and the background as the terminal named them, joined by ";".
const DefaultsEnv = "REWAKE_HOST_COLORS"

// FormatDefaults is the value of DefaultsEnv for the colors a terminal
// answered, empty when it answered neither.
func FormatDefaults(fg, bg string) string {
	if fg == "" && bg == "" {
		return ""
	}
	return fg + ";" + bg
}

// ParseDefaults reads a value of DefaultsEnv; a color it cannot read is nil.
func ParseDefaults(value string) (fg, bg color.Color) {
	f, b, _ := strings.Cut(value, ";")
	return ansi.XParseColor(f), ansi.XParseColor(b)
}
