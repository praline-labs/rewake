package claude

import (
	"strings"

	"github.com/praline-labs/rewake/internal/harness"
)

// addDirFlag gives Claude Code a working directory at launch. A cold resume
// does not bring back a directory the session's hook added, and this flag
// beside --resume does (docs/research-claude-actions.md).
const addDirFlag = "--add-dir"

// ResumedConversation is the conversation --resume or -r names. The value may
// be a search term for the picker rather than an id; then nothing was granted
// in a conversation of that name, and the wrapper finds nothing to restore. A
// fork starts a new conversation, and --continue names none: the wrapper
// learns those from the session's telemetry instead.
func (claudeHarness) ResumedConversation(args []string) string {
	if harness.HasFlag(args, "--fork-session") {
		return ""
	}
	values := harness.FlagValues(args, "--resume", "-r")
	if len(values) == 0 {
		return ""
	}
	value := values[len(values)-1]
	if strings.HasPrefix(value, "-") {
		return ""
	}
	return value
}

// grantDirs adds the directories confirmed again for a resumed conversation.
func grantDirs(args, dirs []string) []string {
	for _, dir := range dirs {
		args = harness.AddFlags(args, addDirFlag, dir)
	}
	return args
}
