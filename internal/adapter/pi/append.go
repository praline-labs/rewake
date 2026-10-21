package pi

import (
	"os"
	"path/filepath"

	"github.com/praline-labs/rewake/internal/adapter"
)

// appendFlag is pi's flag that adds text, or a file's text, to its system
// prompt; repeatable.
const appendFlag = "--append-system-prompt"

// appendSystemFile is the APPEND_SYSTEM.md pi would read itself, which the
// briefing's --append-system-prompt turns off: pi 1.3.0 looks for the file
// only when no such flag is given (resource-loader.js, read October 21,
// 2026), so a project's .pi/APPEND_SYSTEM.md vanished from every run rewake
// started. rewake names it after the briefing as pi would have found it: the
// project's in the working directory when the project is trusted — rewake
// approves it unless the launch says --no-approve — else the agent
// directory's. Empty when there is none, or when the caller's own flag
// already turned discovery off, as pi would have.
func appendSystemFile(callerArgs []string, cwd, agentDir string) string {
	if adapter.HasFlag(callerArgs, appendFlag) {
		return ""
	}
	const name = "APPEND_SYSTEM.md"
	var candidates []string
	if !adapter.HasFlag(callerArgs, "--no-approve") && !adapter.HasFlag(callerArgs, "-na") {
		candidates = append(candidates, filepath.Join(cwd, ".pi", name))
	}
	candidates = append(candidates, filepath.Join(agentDir, name))
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}
