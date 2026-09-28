package codex

import (
	"github.com/praline-labs/rewake/internal/harness"
)

// Remote TUI configuration forwards only selected keys. The server must also
// receive explicit overrides and the briefing without editing the user's file.
func serverConfigArgs(args []string) []string {
	var result []string
	// One parser for the spellings, canonical form on the way out.
	for _, setting := range harness.FlagValues(args, configFlag, "--config") {
		result = append(result, "-c", setting)
	}
	return result
}

// notifyKey is preserved when explicitly configured by the caller. Rewake uses
// server completion events and never installs its own notify program.
const notifyKey = "notify"
