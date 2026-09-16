package cli

import (
	"fmt"
	"sync"

	"github.com/iiiokojiadbi/rewake/internal/harness"
)

// Version is the released version of the tool.
const Version = "0.0.1"

var jsonOption = Option{
	Flag:    "--json",
	Summary: "Print the model behind the output, with every field.",
}

var globalOptions = []Option{
	jsonOption,
	{Flag: "--help", Summary: "Flags, examples and notes for one command."},
	{Flag: "--version", Summary: "Print the version of rewake."},
}

// nameOption is a launch flag, and it goes before the harness name: everything
// after the harness name belongs to the harness.
var nameOption = Option{
	Flag:    "--name",
	Value:   "<name>",
	Summary: "Name this session. Default: the harness name, then claude-2, claude-3.",
}

var (
	groupsOnce sync.Once
	groups     []Group
)

// Groups returns the command table in guide order. Launch commands are derived
// from the harness catalogue, so a registered harness is always runnable and
// always documented.
func Groups() []Group {
	groupsOnce.Do(buildGroups)
	return groups
}

func buildGroups() {
	run := Group{
		Title:   "RUN A SESSION",
		Summary: "Start a coding agent through rewake. It runs in this terminal as usual; rewake only registers it and carries messages.",
	}
	for _, h := range harness.All() {
		run.Commands = append(run.Commands, launchCommand(h))
	}

	talk := Group{
		Title:   "TALK",
		Summary: "Sessions address each other by name. A message arrives as a user message and starts a turn when the receiver is idle.",
		Commands: []*Command{
			{
				Name:           "list",
				MaxPositionals: 0,
				Summary:        "Sessions running under rewake right now.",
				Options:        []Option{jsonOption},
				Examples:       []string{"rewake list", "rewake list --json"},
				Next:           []string{"rewake send <name> \"text\""},
				Handler:        handleList,
			},
			{
				Name:           "send",
				Args:           "<name> <text>",
				MaxPositionals: 2,
				Summary:        "Deliver text to a session. Use - as the text to read it from stdin.",
				Options: []Option{
					{Flag: "--wait", Value: "<seconds>", Summary: "How long to wait for a delivery result. Default: 5."},
					jsonOption,
				},
				Examples: []string{
					"rewake send api \"the migration is merged, pull and rerun the smoke\"",
					"rewake send web - --wait 20",
				},
				Next:    []string{"rewake list"},
				Notes:   []string{"Quote the text as one argument: loose words are refused rather than silently joined."},
				Handler: handleSend,
			},
			{
				Name:           "whoami",
				MaxPositionals: 0,
				Summary:        "The name of this session, when it runs under rewake.",
				Options:        []Option{jsonOption},
				Examples:       []string{"rewake whoami"},
				Handler:        handleWhoami,
			},
		},
	}

	help := Group{
		Title: "HELP",
		Commands: []*Command{
			{
				Name:           "guide",
				MaxPositionals: 0,
				Summary:        "This overview. Printed by rewake with no arguments.",
				Options:        []Option{jsonOption},
				Examples:       []string{"rewake guide"},
				Handler:        handleGuide,
			},
		},
	}

	groups = []Group{run, talk, help}
}

// launchCommand builds the command that starts one harness.
func launchCommand(h harness.Harness) *Command {
	notes := append([]string{}, h.Notes()...)
	return &Command{
		Name:           h.ID(),
		Args:           "[" + h.ID() + " args...]",
		MaxPositionals: Variadic,
		Summary:        h.Summary(),
		Options:        []Option{nameOption, {Flag: "--no-intro", Summary: "Do not tell the agent it runs under rewake."}},
		Examples:       h.Examples(),
		Next:           []string{"rewake list", "rewake send <name> \"text\""},
		Notes:          notes,
		Raw:            true,
		Handler:        handleLaunch(h),
	}
}

// flow is the worked order printed in the guide: real invocations, not shapes.
func flow() []FlowStep {
	steps := []FlowStep{}
	for _, h := range harness.All() {
		steps = append(steps, FlowStep{
			Command: fmt.Sprintf("rewake --name <name> %s", h.ID()),
			Summary: fmt.Sprintf("Start %s in this terminal under a name others can address.", h.Title()),
		})
	}
	return append(steps,
		FlowStep{Command: "rewake list", Summary: "See who is running and can be reached."},
		FlowStep{Command: "rewake send api \"pull and rerun the smoke\"", Summary: "Deliver text; the receiver answers with rewake send."},
		FlowStep{Command: "rewake <command> --help", Summary: "Flags, examples and notes for that command."},
	)
}

// notes describe behaviour that changes how the tool should be called.
func notes() []Note {
	return []Note{
		{
			Title: "Nothing ever prompts",
			Body:  "A CLI that waits for input hangs an agent forever. A missing argument is a refusal that names what was expected and shows a real invocation.",
		},
		{
			Title: "Exit codes are distinguishable",
			Body:  "2 means the call was wrong: unknown command or flag, missing argument, no such session. 1 means the target refused or could not be reached. 3 means a message was accepted but not delivered yet, and will land on its own. Branch on the code instead of parsing text.",
		},
		{
			Title: "Only sessions started through rewake take part",
			Body:  "An agent started by hand in another terminal is not reachable: rewake has no way into it. Start it with rewake and it appears in list.",
		},
		{
			Title: "Delivery speed differs by harness",
			Body:  "Claude Code receives within seconds. Codex polls its queue every ten seconds, and a Codex session that has not exchanged a message yet takes one only after its first turn. Every send says which of these happened.",
		},
	}
}
