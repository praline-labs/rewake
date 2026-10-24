package terminal

import (
	"image/color"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// DefaultsEnv carries the person's terminal's default colors from the shell
// to the space hosts its main opens, so a worker's emulator answers them from
// its start (docs/shell.md): the foreground and the background as the
// terminal named them, joined by ";".
const DefaultsEnv = "REWAKE_HOST_COLORS"

// ParseDefaults reads a value of DefaultsEnv; a color it cannot read is nil.
func ParseDefaults(value string) (fg, bg color.Color) {
	f, b, _ := strings.Cut(value, ";")
	return ansi.XParseColor(f), ansi.XParseColor(b)
}
