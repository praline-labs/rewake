package cli

import (
	"errors"
	"fmt"

	"github.com/praline-labs/rewake/internal/host/mainhost"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// restartCommand restarts one part of rewake around a running main; the
// part is a word, so a later part joins without a new command
// (docs/shell.md#restart-crash-and-close).
func restartCommand() *Command {
	return &Command{
		Name: "restart", Args: "<part>", MaxPositionals: 1,
		Summary:  "Restart one part of rewake while main and its workers keep running. The part is shell: rewake's shell in main's window.",
		Options:  []Option{jsonOption},
		Examples: []string{"rewake restart shell", "rewake restart shell --json"},
		Next:     []string{"rewake screen main"},
		Notes: []string{
			"Any session of the room may ask, and the person has the same with Ctrl+\\ then r. Main's host in the person's pane starts a new shell, resolved as a launch resolves it (REWAKE_SHELL, or rewake-shell beside rewake), and lets the old one go only once the new one has answered; the new shell attaches to main and to every open space from their emulators. Keys typed during the swap are dropped, not kept for the new shell.",
			"Returns once the new shell draws, with its generation: the first shell of the launch is 1. A new shell that does not start leaves the old one running; one that starts and then fails leaves main alone in the plain terminal, and a second restart tries again. Main and its workers keep running either way.",
			"Exit 0 restarted; 1 refused or failed (no host in the room — main runs plain or not at all —, rewake closing, main ending, a restart already under way, the new shell did not start); 2 a wrong call — a part rewake does not have.",
		},
		Access: changeForm, Handler: handleRestart,
	}
}

// restartView is the answer: the part, and the generation now running.
type restartView struct {
	Part       string `json:"part"`
	Generation int    `json:"generation"`
}

func handleRestart(ctx *Context, call Call) error {
	part := call.Positionals[0]
	if part != "shell" {
		return &UsageError{Command: call.Command, Message: fmt.Sprintf("no part %q to restart: the parts are shell.", part)}
	}
	dir, err := state.Dir()
	if err != nil {
		return &UsageError{Command: call.Command, Message: err.Error()}
	}
	generation, err := mainhost.Ask(mainhost.HostSocket(dir), part)
	if errors.Is(err, mainhost.ErrNoHost) {
		return failf("no host answers in this room: main runs without rewake's shell, or no main runs, so there is no shell to restart")
	}
	if err != nil {
		return failf("the %s was not restarted: %v", part, err)
	}
	view := restartView{Part: part, Generation: generation}
	return printValue(ctx, view, func() []string {
		return []string{"Rewake: the " + part + " restarted; generation " + itoa(generation) + " draws now."}
	})
}
