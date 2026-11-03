package cli

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/praline-labs/rewake/internal/host/mainhost"
	"github.com/praline-labs/rewake/internal/infra/parts"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// restartCommand restarts one part of rewake around a running main, or with
// no part says what the room needs restarted; the part is a word, so a later
// part joins without a new command (docs/shell.md#restart-crash-and-close,
// docs/install.md#the-parts-and-what-an-update-restarts).
func restartCommand() *Command {
	return &Command{
		Name: "restart", Args: "[<part>]", MaxPositionals: 1,
		Summary:  "Restart one part of rewake while main and its workers keep running: shell, rewake's shell in main's window; face, the face's code reloaded in place; or rewake, the running room switched to the installed rewake. With no part, what the room needs restarted to run what is installed.",
		Options:  []Option{jsonOption},
		Examples: []string{"rewake restart", "rewake restart shell", "rewake restart face", "rewake restart rewake", "rewake restart shell --json"},
		Next:     []string{"rewake screen main"},
		Notes: []string{
			"With no part it compares the parts the room runs — this rewake, the shell main's host draws with — with the manifest of the install, and prints one line per part to restart: what changed and what it takes. live: the part reloads itself; shell: rewake restart shell; sessions: each new session takes it; switch: rewake restart rewake, while every session runs on; room: end main and start it again, which ends its workers — a new rewake that changes a contract a switch carries. A part may also need a restart because a contract it requires or speaks changed. An install without a manifest — a build by hand — has no plan.",
			"Any session of the room may ask, and the person has the same with Ctrl+\\ then r. Main's host in the person's pane starts a new shell, resolved as a launch resolves it (REWAKE_SHELL, or rewake-shell beside rewake), and lets the old one go only once the new one has answered; the new shell attaches to main and to every open space from their emulators. Keys typed during the swap are dropped, not kept for the new shell.",
			"Returns once the new shell draws, with its generation: the first shell of the launch is 1. A new shell that does not start leaves the old one running; one that starts and then fails leaves main alone in the plain terminal, and a second restart tries again. Main and its workers keep running either way. face reloads every face from its files, as the shell's watch of them does.",
			"rewake switches the running room to the installed rewake without ending a session: each session and host is asked whether it can hand its run over, and when all can and are idle, each hands over, the room's build becomes the installed one, and each takes its run up again on it. It is refused, the room keeping its build, when one cannot hand over — Claude Code without its mod —, when one stays busy for a minute, or when the installed rewake speaks or requires a contract at another value; restart the room then. With the installed rewake the room's own it says nothing to switch. Asked from a session — main's own turn, which keeps main busy — it returns at once and the switch runs once every session is idle, tried again every 5 s for 10 minutes and given up if main ends; the outcome comes to main as a note. Asked by the person, it waits for the outcome and prints it. One deferred switch waits at a time.",
			"A harness rewake ships, which only a restart of the room loads, is refused, naming it; so is a shell whose installed copy requires a contract the room's rewake does not speak.",
			"Exit 0 restarted, switched, or the plan printed; 1 refused or failed (no host in the room — main runs plain or not at all —, rewake closing, main ending, a restart already under way, the new shell did not start, a part only the room's restart loads, a switch refused or a session lost after it); 2 a wrong call — a part rewake does not have.",
		},
		Access: changeForm, Handler: handleRestart,
	}
}

// restartView is the answer: the part, and the generation now running.
type restartView struct {
	Part       string `json:"part"`
	Generation int    `json:"generation"`
}

// planView is the answer with no part: each part to restart, or why there
// is no plan.
type planView struct {
	Steps   []parts.Step `json:"steps"`
	Problem string       `json:"problem,omitempty"`
}

func handleRestart(ctx *Context, call Call) error {
	update := roomUpdate(ctx.table.catalog, installedDir())
	if len(call.Positionals) == 0 {
		return printPlan(ctx, update)
	}
	part := call.Positionals[0]
	names := partNames(ctx.table.catalog, update.installed)
	if !slices.Contains(names, part) {
		return &UsageError{Command: call.Command, Message: fmt.Sprintf("no part %q to restart: the parts are %s.", part, strings.Join(names, ", "))}
	}
	if part == parts.Rewake {
		return restartRewake(ctx)
	}
	if !parts.InPlace(part) {
		installed, listed := update.installed.Find(part)
		return failf("%s", parts.NotInPlace(part, installed, listed))
	}
	if why := parts.Blocks(update.running, update.installed, part); why != "" {
		return failf("the %s was not restarted: %s; restart the room — end main and start it again — to load both", part, why)
	}
	dir, err := state.Dir()
	if err != nil {
		return &UsageError{Command: call.Command, Message: err.Error()}
	}
	generation, err := mainhost.Ask(mainhost.HostSocket(dir), part)
	if errors.Is(err, mainhost.ErrNoHost) {
		return failf("no host answers in this room: main runs without rewake's shell, or no main runs, so there is no %s to restart", part)
	}
	if err != nil {
		return failf("the %s was not restarted: %v", part, err)
	}
	view := restartView{Part: part, Generation: generation}
	return printValue(ctx, view, func() []string {
		if part == parts.Face {
			return []string{"Rewake: the faces reloaded in shell generation " + itoa(generation) + "."}
		}
		return []string{"Rewake: the " + part + " restarted; generation " + itoa(generation) + " draws now."}
	})
}

func printPlan(ctx *Context, update updateOf) error {
	view := planView{Steps: update.steps, Problem: update.problem}
	if view.Steps == nil {
		view.Steps = []parts.Step{}
	}
	return printValue(ctx, view, func() []string {
		switch {
		case update.problem != "":
			return []string{"Rewake: no plan: " + update.problem + "."}
		case len(update.steps) == 0:
			return []string{"Rewake: nothing to restart; the room runs what is installed."}
		}
		lines := make([]string, 0, len(update.steps))
		for _, step := range update.steps {
			lines = append(lines, step.Line())
		}
		return lines
	})
}
