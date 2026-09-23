package cli

import (
	"fmt"
	"sync"

	"github.com/iiiokojiadbi/rewake/internal/alias"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/role"
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
	Value:   "<prefix>",
	Summary: "Session prefix (default: selected role, general without a role flag). Address: <prefix>-<harness>, up to 32 characters. Automatic conflicts add -2, -3; explicit conflicts refuse.",
}

// commandOption names the program a launch starts instead of the harness's
// own: a person's wrapper script, which sets up an environment and then runs
// the harness. The harness word still says which harness it is — rewake does
// not guess that from a program's name, and does not run the program to ask.
var commandOption = Option{
	Flag: "--command", Value: "<program>",
	Summary: "Start this program instead of the harness's own, with everything rewake adds unchanged: a wrapper script that runs the harness. A name is looked up on PATH, a path with a slash is taken as given; either must be an executable file.",
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
		Summary: "Sessions address each other by name. Nearby incoming messages share a notice. Use rewake inbox --peek for IDs and previews, --message <id> to read one, or plain inbox to read all.",
		Commands: []*Command{
			{
				Name:           "list",
				MaxPositionals: 0,
				Summary:        "Live sessions in an aligned table; room and shared directory appear once.",
				Options:        []Option{jsonOption},
				Examples:       []string{"rewake list", "rewake list --json"},
				Next:           []string{"rewake send <name> \"text\""},
				Notes:          []string{sessionStateHelp, "Main receives a notify when a session becomes available; a later main also learns which sessions were already available. Each launch epoch is announced once."},
				Handler:        handleList,
			},
			{
				Name:           "send",
				Args:           "<name> <text>",
				MaxPositionals: 2,
				Summary:        "Give a session a task, a question or a heads-up. Use - as the text to read it from stdin.",
				Options: append(kindOptions(),
					Option{Flag: "--grant-git", Summary: "Verified main only: explicitly grant eligible task/question recipients access to validated repository Git metadata. No flag adds no roots."},
					Option{Flag: "--wait", Value: "<seconds>", Summary: "How long to wait. Default: 5 for the delivery, 600 for a question's answer."},
					jsonOption,
				),
				Examples: append(sendExamples(), "rewake send writer-codex --grant-git \"Commit the reviewed change\""),
				Next:     []string{"rewake inbox"},
				Notes: []string{
					"A task is the default: the session reads it, works, and ends its turn with a final message, which comes back to you as a \"Rewake: <session> finished\" line.",
					"A question blocks until that final message and prints it. A long one is better run in the background.",
					sessionStateHelp,
					"Quote the text as one argument: loose words are refused rather than silently joined.",
				},
				Handler: handleSend,
			},
			{
				Name:           "inbox",
				MaxPositionals: 0,
				Summary:        "Read waiting messages, or preview their metadata without consuming them.",
				Options: []Option{
					jsonOption,
					{Flag: "--peek", Summary: "Show IDs, senders, kinds, times and bounded first-line previews only; no messages are marked read."},
					{Flag: "--message", Value: "<id>", Summary: "Read only this available unread message; reserved answers remain with their waiting send."},
				},
				Examples: []string{"rewake inbox", "rewake inbox --json", "rewake inbox --peek", "rewake inbox --peek --json", "rewake inbox --message=1780000000000000000-012345abcdef"},
				Next:     []string{"rewake send <name> \"text\""},
				Notes: []string{
					"--peek and --message are mutually exclusive. Peek has no full bodies, even in JSON, and creates no task read receipts or report obligations. Plain inbox still reads all available messages.",
					"Run it when a Rewake notice says messages are waiting; a group may mix tasks, questions, notifications and reports.",
					"A task or a question you read is answered by ending your turn: your final message goes back to the sender by itself. Put the result there.",
					"A notify needs no answer. A finished message is a session's final message after work you gave it.",
					sessionStateHelp,
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
			{
				Name:     harness.Observe,
				Args:     "<socket>",
				Summary:  "Called by a Claude Code hook; passes what the hook was handed to the session's wrapper.",
				Examples: []string{"rewake observe /tmp/rewake-1000/rooms/default/sock/worker-claude.1.obs"},
				Raw:      true,
				Hidden:   true,
				Handler:  handleObserve,
			},
			{
				Name:     harness.StatusTap,
				Args:     "<socket> [sources] [caller-status-line]",
				Summary:  "The status line of a Claude Code session; reports to its wrapper, then runs the configured status line.",
				Examples: []string{"rewake status-tap /tmp/rewake-1000/rooms/default/sock/worker-claude.1.obs user,project,local"},
				Raw:      true,
				Hidden:   true,
				Handler:  handleStatusTap,
			},
		},
	}

	groups = []Group{run, talk, help, internal}
}

// launchCommand builds the command that starts one harness.
func launchCommand(h harness.Harness) *Command {
	notes := append([]string{}, h.Notes()...)
	notes = append(notes, harness.SettingsHelp, alias.Help)
	return &Command{
		Name:           h.ID(),
		Args:           "[" + h.ID() + " args...]",
		MaxPositionals: Variadic,
		Summary:        h.Summary(),
		Options: append(append([]Option{nameOption, roomOption, commandOption}, roleOptions()...),
			Option{Flag: "--no-intro", Summary: "Do not add the system-layer briefing."},
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
			Command: fmt.Sprintf("rewake --%s --name helper %s", role.General.ID, h.ID()),
			Summary: fmt.Sprintf("Start %s as helper-%s with role %s; --name sets only the prefix.", h.Title(), h.ID(), role.General.ID),
		}, FlowStep{
			Command: fmt.Sprintf("rewake send helper-%s \"pull and rerun the smoke\"", h.ID()),
			Summary: "Give that exact address a task; its final message comes back when the turn ends.",
		})
	}
	return append(steps,
		FlowStep{Command: "rewake list", Summary: "See who is running and can be reached."},
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
			Body:  "It shows up as one line: \"Rewake: <session> <kind>, <n> new message(s)\", with a 🟢 in front where the harness shows it as plain text. The following line previews the author's first line, limited to about 100 columns. Each notice has fixed member IDs. Ready new mail is submitted promptly through native start-or-steer, without waiting for peek or a completed turn. No reminders for old unread mail. Initial collection is 150 ms; later arrivals join the next available dispatch, never an already accepted notice. Git grants accompany only the actual eligible task announcement. Use rewake inbox --peek for a non-consuming overview, --message <id> for one full message, or plain inbox for all. Groups preserve separate identities and obligations. Start every message and final reply with one line stating its point. Errors use a red circle; keyboard stops use yellow.",
		},
		{
			Title: "Kinds and replies",
			Body:  kindSummary(),
		},
		{
			Title: "Rooms isolate conversations",
			Body:  "Choose --room <name> before the harness name; without it, launches use default. The system briefing names the role and directs the agent to rewake guide on its first task. Sessions see only their room. Commands inherit REWAKE_ROOM; a shell without it uses default. Names can repeat across rooms. There is no cross-room address or --room flag on messaging commands.",
		},
		{
			Title: "Session addresses come from launch prefixes",
			Body:  "The selected role is the default prefix; --name replaces that prefix without changing the role. Launch always appends -<harness>, even if the prefix already ends with that suffix. Automatic conflicts add -2, -3 after the harness; explicit conflicts refuse. Prefixes start with a lower-case letter or digit and use lower-case letters, digits, dots, dashes or underscores; the complete address must fit 32 characters. Send uses the exact address shown by list. Existing running sessions keep their names.",
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
