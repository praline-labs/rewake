package codex

import (
	"strings"

	"github.com/iiiokojiadbi/rewake/internal/harness"
)

// Remote TUI configuration forwards only selected keys. The server must also
// receive explicit overrides and the briefing without editing the user's file.
func serverConfigArgs(args []string) []string {
	var result []string
	visible := harness.BeforeTerminator(args)
	for i := 0; i < len(visible); i++ {
		arg := visible[i]
		switch {
		case arg == "-c" || arg == "--config":
			if i+1 < len(visible) {
				result = append(result, "-c", visible[i+1])
				i++
			}
		case strings.HasPrefix(arg, "--config="):
			result = append(result, "-c", strings.TrimPrefix(arg, "--config="))
		case strings.HasPrefix(arg, "-c") && len(arg) > 2:
			result = append(result, "-c", strings.TrimPrefix(arg[2:], "="))
		}
	}
	return result
}

// notifyKey is preserved when explicitly configured by the caller. Rewake uses
// server completion events and never installs its own notify program.
const notifyKey = "notify"
