package cli

import (
	"fmt"
	"sync"

	"github.com/iiiokojiadbi/rewake/internal/harness"
)

// Version is the released version of the tool. A release build sets it from the
// package version, so the two cannot disagree.
var Version = "0.0.1"

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

var roomOption = Option{
	Flag: "--room", Value: "<name>",
	Summary: "Join this room at launch. Default: default. Names are unique within a room.",
}

var (
	groupsOnce sync.Once
	groups     []Group
)

// Groups returns the command table in guide order. Launch commands are derived
// from the harness catalog, so a registered harness is always runnable and
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
		Summary: "Sessions address each other by name. The receiver sees a message notice and a bounded first-line preview, \"Rewake: <session> <kind>, <n> new message(s)\", and reads it with rewake inbox.",
		Commands: []*Command{
			{
				Name:           "list",
				MaxPositionals: 0,
				Summary:        "Live sessions in this room, with their room and role.",
				Options:        []Option{jsonOption},
				Examples:       []string{"rewake list", "rewake list --json"},
				Next:           []string{"rewake send <name> \"text\""},
				Handler:        handleList,
			},
			{
				Name:           "send",
				Args:           "<name> <text>",
				MaxPositionals: 2,
				Summary:        "Give a session a task, a question or a heads-up. Use - as the text to read it from stdin.",
				Options: append(kindOptions(),
					Option{Flag: "--wait", Value: "<seconds>", Summary: "How long to wait. Default: 5 for the delivery, 600 for a question's answer."},
					jsonOption,
				),
				Examples: []string{
					"rewake send api \"rerun the smoke and report what failed\"",
					"rewake send web \"which port does the dev server use?\" --question",
					"rewake send api \"the migration is merged\" --notify",
					"rewake send web - --wait 20",
				},
				Next: []string{"rewake inbox"},
				Notes: []string{
					"A task is the default: the session reads it, works, and ends its turn with a final message, which comes back to you as a \"Rewake: <session> finished\" line.",
					"A question blocks until that final message and prints it. A long one is better run in the background.",
					"Quote the text as one argument: loose words are refused rather than silently joined.",
				},
				Handler: handleSend,
			},
			{
				Name:           "inbox",
				MaxPositionals: 0,
				Summary:        "Read the messages waiting for this session. Each is shown once.",
				Options:        []Option{jsonOption},
				Examples:       []string{"rewake inbox", "rewake inbox --json"},
				Next:           []string{"rewake send <name> \"text\""},
				Notes: []string{
					"Run it when a \"Rewake: <session> <kind>\" line says messages are waiting.",
					"A task or a question you read is answered by ending your turn: your final message goes back to the sender by itself. Put the result there.",
					"A notify needs no answer. A finished message is a session's final message after work you gave it.",
				},
				Handler: handleInbox,
			},
			{
				Name:           "whoami",
				MaxPositionals: 0,
				Summary:        "The name, room and role of this session, when it runs under rewake.",
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

	internal := Group{
		Title: "INTERNAL",
		Commands: []*Command{
			{
				Name:           "turn-ended",
				Args:           "[payload]",
				MaxPositionals: 1,
				Summary:        "Called by a harness when a turn ends; tells the sessions that wrote here.",
				Examples:       []string{"rewake turn-ended"},
				Hidden:         true,
				Handler:        handleTurnEnded,
			},
		},
	}

	groups = []Group{run, talk, help, internal}
}

// launchCommand builds the command that starts one harness.
func launchCommand(h harness.Harness) *Command {
	notes := append([]string{}, h.Notes()...)
	return &Command{
		Name:           h.ID(),
		Args:           "[" + h.ID() + " args...]",
		MaxPositionals: Variadic,
		Summary:        h.Summary(),
		Options: append(append([]Option{nameOption, roomOption}, roleOptions()...),
			Option{Flag: "--no-intro", Summary: "Do not add the system-layer briefing."},
			Option{Flag: "--no-greeting", Summary: "Do not start a fresh conversation with the guide-and-ready prompt."},
		),
		Examples: append(h.Examples(), "rewake --room work --general --name helper "+h.ID()),
		Next:     []string{"rewake list", "rewake send <name> \"text\""},
		Notes:    notes,
		Raw:      true,
		Harness:  h,
		Handler:  handleLaunch(h),
	}
}

// flow is the worked order printed in the guide: real invocations, not shapes.
func flow() []FlowStep {
	steps := []FlowStep{}
	for _, h := range harness.All() {
		steps = append(steps, FlowStep{
			Command: fmt.Sprintf("rewake --name <name> %s", h.ID()),
			Summary: fmt.Sprintf("Start %s in this terminal under a name others can address; the room elects main automatically, or use --general, --write or --main.", h.Title()),
		})
	}
	return append(steps,
		FlowStep{Command: "rewake list", Summary: "See who is running and can be reached."},
		FlowStep{Command: "rewake send api \"pull and rerun the smoke\"", Summary: "Give api a task; its final message comes back as a \"Rewake: api finished\" line."},
		FlowStep{Command: "rewake inbox", Summary: "When a \"Rewake:\" line says messages are waiting, read them here. Answer a task by finishing your turn with the result."},
		FlowStep{Command: "rewake <command> --help", Summary: "Flags, examples and notes for that command."},
	)
}

// notes describe behavior that changes how the tool should be called.
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
			Title: "A waiting message is announced, not pasted",
			Body:  "It shows up as one line: \"Rewake: <session> <kind>, <n> new message(s)\", with a 🟢 in front where the harness shows it as plain text. The following line previews the author's first line, limited to about 100 columns. Run rewake inbox for the full text. Start every message and final reply with one line stating its point. Errors use a red circle; keyboard stops use yellow.",
		},
		{
			Title: "Kinds and replies",
			Body:  kindSummary(),
		},
		{
			Title: "Rooms isolate conversations",
			Body:  "Choose --room <name> before the harness name; without it, launches use default. Fresh conversations first run rewake guide and reply ready; --no-greeting disables that first turn independently of --no-intro. Sessions see only their room. Commands inherit REWAKE_ROOM; a shell without it uses default. Names can repeat across rooms. There is no cross-room address or --room flag on messaging commands.",
		},
		{
			Title: "Choose a session role",
			Body:  roleSummary(),
		},
		{
			Title: "A message from shell cannot be answered with send",
			Body:  "shell means it was typed in a plain terminal, not sent by a session. Answer it in your own reply; rewake has no way to deliver to it.",
		},
		{
			Title: "Delivery speed differs by harness",
			Body:  "Claude Code receives through its inbox socket. Codex uses an owned app-server to start or steer a turn, including in a fresh conversation. Every send reports acceptance or the reason delivery failed.",
		},
	}
}
