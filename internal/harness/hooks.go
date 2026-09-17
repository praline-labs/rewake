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
	quoted := make([]string, 0, len(argv))
	for _, arg := range argv {
		quoted = append(quoted, "'"+strings.ReplaceAll(arg, "'", `'\''`)+"'")
	}
	return strings.Join(quoted, " "), nil
}
