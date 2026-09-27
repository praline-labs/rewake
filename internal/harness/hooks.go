package harness

import (
	"fmt"
	"os"
	"strings"
)

// TurnEnded is the hidden command a harness calls when a turn of its session
// ends. It is not in the guide: agents have no reason to run it.
const TurnEnded = "turn-ended"

// TurnEndedArgv is the command a harness runs at the end of a turn, by absolute
// path: the hook runs with whatever PATH the harness has at that moment.
func TurnEndedArgv() ([]string, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("could not find the rewake binary: %w", err)
	}
	return []string{executable, TurnEnded}, nil
}

// TurnEndedCommand is TurnEndedArgv as one shell command line.
func TurnEndedCommand() (string, error) {
	argv, err := TurnEndedArgv()
	if err != nil {
		return "", err
	}
	return ShellQuote(argv), nil
}

// ShellQuote spells an argv as one POSIX shell command line: every argument in
// single quotes, so a path with spaces or a command with its own quoting
// arrives as the same argument it was.
func ShellQuote(argv []string) string {
	quoted := make([]string, 0, len(argv))
	for _, arg := range argv {
		quoted = append(quoted, "'"+strings.ReplaceAll(arg, "'", `'\''`)+"'")
	}
	return strings.Join(quoted, " ")
}

// Observe and StatusTap are the hidden commands a Claude Code session runs for
// telemetry: one from its hooks, one as its status line.
const (
	Observe   = "observe"
	StatusTap = "status-tap"
)

// GrantHook is the hidden command a Claude Code session runs before a tool
// call and on a permission request: it gives and takes back the directories
// granted to that session (docs/grants.md).
const GrantHook = "grant-hook"

// GrantRewakeRule is the switch the grant hook's command carries when the
// launch added the allow rule for rewake itself. Only then does a plain rewake
// command run unasked, so only then may the question forced on it be answered
// with a grant's removal: a person who gave their own allowed tools may have
// left rewake out of them.
const GrantRewakeRule = "rewake-allowed"
