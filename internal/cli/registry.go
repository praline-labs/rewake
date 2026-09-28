package cli

import (
	"fmt"
	"sync"

	"github.com/praline-labs/rewake/internal/alias"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/role"
)

// Version is the released version of the tool. A release build sets it from the
// package version, so the two cannot disagree.
var Version = "1.0.0"

var jsonOption = Option{
	Flag:    "--json",
	Summary: "Print the model behind the output, with every field.",
}

var globalOptions = []Option{
	jsonOption,
	{Flag: "--help", Summary: "Flags, examples and notes for one command."},
	{Flag: "--version", Summary: "The version and its build: revision, build or commit time, modified."},
}

// nameOption is a launch flag, and it goes before the harness name: everything
// after the harness name belongs to the harness.
var nameOption = Option{
	Flag:    "--name",
	Value:   "<prefix>",
	Summary: "Address prefix, the role by default: the session is <prefix>-<harness>, 32 characters at most. A taken default gets -2, -3; a taken --name is refused.",
}

// commandOption names the program a launch starts instead of the harness's
// own: a person's wrapper script, which sets up an environment and then runs
// the harness. The harness word still says which harness it is — rewake does
// not guess that from a program's name, and does not run the program to ask.
var commandOption = Option{
	Flag: "--command", Value: "<program>",
	Summary: "Start this program, a wrapper script that runs the harness, in place of the harness with the same arguments. A name is looked up on PATH, a path with a slash taken as given; it must be executable.",
}

var roomOption = Option{
	Flag: "--room", Value: "<name>",
	Summary: "The room to join; default otherwise. Names are unique within a room.",
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
		Summary: "A coding agent started through rewake runs in this terminal as usual; rewake registers it and carries its messages.",
	}
	for _, h := range harness.All() {
		run.Commands = append(run.Commands, launchCommand(h))
	}
	run.Commands = append(run.Commands, worktreeCommand())

	talk := Group{
		Title:   "TALK",
		Summary: "Sessions address each other by name. A notify or a report waits a few seconds to share one notice with other mail; a task or a question is announced at once.",
		Commands: []*Command{
			{
				Name:           "list",
				MaxPositionals: 0,
				Summary:        "The running sessions of this room.",
				Options:        []Option{jsonOption},
				Examples:       []string{"rewake list", "rewake list --json"},
				Next:           []string{"rewake send <name> \"text\""},
				Notes:          []string{sessionStateHelp, "Main is told once per launch when a session becomes available, and when one compacts or leaves; a main started later learns which are already available. Idleness alone resends nothing."},
				Handler:        handleList,
			},
			sendCommand(),
			{
				Name:           "withdraw",
				Args:           "<id>",
				MaxPositionals: 1,
				Summary:        "Take back a message this run sent, while it is unread.",
				Options:        []Option{jsonOption},
				Examples:       []string{"rewake withdraw 8d4ddd85", "rewake withdraw 1790370984617481194-8d4ddd85c8f1 --json"},
				Next:           []string{"rewake inbox --awaited"},
				Notes: []string{
					"Before its notice went out, the message just goes. After, it stays in the recipient's inbox marked withdrawn under the same id, and a note tells the recipient at once not to act on the notice, which it may have acted on from the preview alone. A notice held for approval stays in the harness's queue, which rewake cannot empty, and leads to the withdrawn mark if approved. A send blocked on a withdrawn question exits 1.",
					"Only the run that sent it may, in any role; a plain shell's or an earlier run's mail is out of reach. A read message is final: add to it with rewake send <name> \"...\" --to <id>.",
					"A task's unread addenda are withdrawn with it. A read addendum stays owed, and the task is recalled for it even if the task's own notice never went out.",
					"An id rewake edit replaced stands for its replacement; the output says so first.",
					"A withdrawal cut short says how far it got and the command that finishes it. Exit 0 withdrawn, now or before; 1 read, not delivered, no longer kept, no such message, or not finished; 2 a wrong call — not a session, an id too short.",
				},
				Handler: handleWithdraw,
			},
			{
				Name:           "edit",
				Args:           "<id> <text>",
				MaxPositionals: 2,
				Summary:        "Replace an unread message this run sent with a new text of the same kind. Use - as the text to read it from stdin.",
				Options: []Option{
					{Flag: "--wait", Value: "<seconds>", Summary: "How long to wait, as for send."},
					jsonOption,
				},
				Examples: []string{"rewake edit 8d4ddd85 \"rerun the smoke on the staging branch\"", "rewake edit 8d4ddd85 - --json"},
				Next:     []string{"rewake inbox --awaited"},
				Notes: []string{
					"One step withdraws the old message and sends the new one: the recipient finds the old one marked withdrawn and replaced, and the new one's notice names the old as withdrawn, so no separate note goes. The output is send's, ending with the new id.",
					"A question's replacement waits for its answer as send --question does; the send blocked on the old one exits 1.",
					"A task's addenda stay and add to the replacement; the output names them, with the command to take one back.",
					"An id an earlier edit replaced stands for its replacement; the output says so first.",
					"Who may, and when, as for rewake withdraw; exit codes as for send.",
				},
				Handler: handleEdit,
			},
			{
				Name:           "inbox",
				MaxPositionals: 0,
				Summary:        "Read waiting messages, or preview them, show again what you owe a report for, or list what others owe you.",
				Options: []Option{
					jsonOption,
					{Flag: "--peek", Summary: "IDs, senders, kinds, times and first-line previews only; nothing is marked read."},
					{Flag: "--message", Value: "<id>", Summary: "Read only this unread message; an answer reserved for a waiting send stays with it."},
					{Flag: "--owed", Summary: "Show again, in full, the tasks and questions you read and have not reported on; changes nothing."},
					{Flag: "--awaited", Summary: "The tasks and questions this run sent that have no report yet, by recipient, with where each stands; changes nothing."},
				},
				Examples: []string{"rewake inbox", "rewake inbox --json", "rewake inbox --peek", "rewake inbox --peek --json", "rewake inbox --message=1780000000000000000-012345abcdef", "rewake inbox --owed", "rewake inbox --owed --json", "rewake inbox --awaited", "rewake inbox --awaited --json"},
				Next:     []string{"rewake send <name> \"text\""},
				Notes: []string{
					"--peek and --message exclude each other. A peek shows no bodies, even in JSON, and creates no read receipt or report obligation.",
					"--owed goes alone: after a context compaction, re-read your task there rather than from the summary. It shows only what you read; a last line counts what is still unread (unread in --json), which plain inbox reads. Refused for main, which owes no reports. A task from a plain shell owes none and is not listed.",
					"--awaited goes alone, in any role; after a compaction, main runs it to see what it is still owed. Each message shows its id, kind, time, first line and state: not delivered yet, held, delivered and unread, read, pending, or stopped, with who stopped it — the keyboard, or a main by name. A recipient that ended owing a read task reads \"<name> ended; a resume of <name> in its conversation may still report\" for a day from the read: a resume under that name takes the task over, so do not resend it yet. After that day, once a new run of the name in another conversation swept it, for a task never read, or for a recipient replaced by a new run, it reads \"no report coming\". --json carries the full text. Only this run's tasks and questions are listed.",
					"Answer a task or a question by ending your turn with the result: the final message goes back to the sender by itself. Nothing else is answered.",
					"A verified main sees the sender's state line above each of its messages; rewake list --help says what it holds.",
				},
				Handler: handleInbox,
			},
			{
				Name:           "pending",
				Args:           "<text>",
				MaxPositionals: 1,
				Summary:        "Mark a turn that ends before the work does, saying what it waits for: the senders hear the work is still going, and a later turn reports.",
				Options:        []Option{jsonOption},
				Examples:       []string{"rewake pending \"the suite is running; the report follows when it ends\""},
				Notes: []string{
					"Without it, the turn's end is the report and the sender stops waiting. Waiting on background work inside the turn beats both.",
					"It marks only the turn it runs in, and only its normal end: a failed or stopped turn reports as usual.",
					"On Claude Code, an unmarked turn end after a pending one is held once to ask whether the work is done: still waiting — run rewake pending and end the turn; done — end the turn, and the answer already given goes into the report with what follows.",
					"Refused outside a session; for main, whose turns go to nobody; with no read task or question awaiting a report; and on a Claude Code session whose telemetry hooks record no turn start to tie the mark to.",
				},
				Handler: handlePending,
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

	steer := Group{
		Title:   "STEER A SESSION",
		Summary: "Main only, on a running session of its room.",
		Commands: []*Command{
			{
				Name:           "compact",
				Args:           "<name> [focus]",
				MaxPositionals: 2,
				Summary:        "Compact an idle session's conversation now, optionally telling the summary what to keep.",
				Options:        []Option{jsonOption},
				Examples:       []string{"rewake compact worker-claude", "rewake compact worker-claude \"keep the review findings and the open questions\"", "rewake compact worker-claude --json"},
				Next:           []string{"rewake list"},
				Notes: []string{
					"Refused at once while the session is in a turn; nothing waits for the turn to end. Interrupt it first, or ask again once rewake list shows it idle.",
					"A focus is refused for Codex before anything is sent: it has no way to pass one.",
					"Waits up to 5 seconds for the session to take the request — its rewake plugin on Claude Code, its wrapper on Codex — then up to 10 for its answer, and returns once the compaction has started. \"requested\" means the start was not seen in time and may still come. The result then arrives as a notify from the session, in place of the \"context compacted\" notice: token counts before and after where the harness gives them and its count of compactions, or why it was refused or failed; never the summary.",
					"Exit 0 started or requested; 1 refused (in a turn, compaction switched off, nothing to compact, remote conversation, not answering, cut short, no control directory, withdrawn before it was taken, another request in flight) or failed; 2 a wrong call — not a main, no such session, a harness that does not take it or cannot take a focus.",
				},
				Handler: handleCompact,
			},
			{
				Name:           "interrupt",
				Args:           "<name>",
				MaxPositionals: 1,
				Summary:        "Stop the turn a session is working on, as Esc would.",
				Options:        []Option{jsonOption},
				Examples:       []string{"rewake interrupt worker-claude", "rewake interrupt worker-claude --json"},
				Next:           []string{"rewake inbox --awaited"},
				Notes: []string{
					"A session waiting on that turn reads stopped, naming who interrupted it. On Claude Code the session's next notice tells it once that you interrupted its turn; Codex records the interrupt in its model's history itself.",
					"Refused when no turn runs. Waits as rewake compact does: 5 seconds for the session to take the request, then 10 for its answer.",
					"Exit 0 done; 1 refused (no turn running, not answering, cut short, no control directory, withdrawn before it was taken, another request in flight) or failed; 2 a wrong call — not a main, no such session, a harness that does not take it.",
				},
				Handler: handleInterrupt,
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
				Summary:  "Called by a Claude Code hook or rewake's plugin; passes what it was handed to the session's wrapper.",
				Examples: []string{"rewake observe /tmp/rewake-1000/rooms/default/sock/worker-claude.1.obs"},
				Raw:      true,
				Hidden:   true,
				Handler:  handleObserve,
			},
			{
				Name:    harness.GrantHook,
				Summary: "Called by a Claude Code hook before a tool call and on a permission request; gives and takes back directories granted with a task.",
				Options: []Option{{
					Flag:    "--" + harness.GrantRewakeRule,
					Summary: "The launch added the allow rule for rewake, so a plain rewake command may take a grant back.",
				}},
				Examples: []string{"rewake grant-hook --" + harness.GrantRewakeRule},
				Hidden:   true,
				Handler:  handleGrantHook,
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

	groups = []Group{run, talk, steer, help, internal}
}

// nestedLaunchHelp is refuseNestedLaunch's rule, on every launch page: a
// main reading how to start a worker must not learn it only from the refusal.
const nestedLaunchHelp = "Refused with exit 2 from a shell inside a rewake session: the owner starts sessions, from a terminal outside any."

// launchCommand builds the command that starts one harness.
func launchCommand(h harness.Harness) *Command {
	notes := append([]string{}, h.Notes()...)
	notes = append(notes, nestedLaunchHelp, harness.SettingsHelp, alias.Help)
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
			Summary: "Give that exact address a task; its report comes back when the turn ends.",
		})
	}
	// The id a send prints, or its tail, names the message afterwards.
	last := harness.All()[len(harness.All())-1].ID()
	return append(steps,
		FlowStep{
			Command: fmt.Sprintf("rewake send helper-%s \"also rerun the lint\" --to 8d4ddd85", last),
			Summary: "Add to an unreported task by the id its send printed, or a unique prefix; one report settles both.",
		},
		FlowStep{
			Command: "rewake edit 8d4ddd85 \"pull and rerun the full suite\"",
			Summary: "Replace an unread message of yours; rewake withdraw 8d4ddd85 takes it back instead.",
		},
		FlowStep{Command: "rewake list", Summary: "See who is running and can be reached."},
		FlowStep{Command: "rewake inbox", Summary: "Read what a \"Rewake:\" notice announced; answer a task by ending your turn with the result."},
	)
}

// notes describe behavior that changes how the tool should be called.
func notes() []Note {
	return []Note{
		{
			Title: "Nothing ever prompts",
			Body:  "A CLI that waits for input hangs an agent. A missing argument is a refusal naming what was expected, with a real invocation.",
		},
		{
			Title: "Exit codes are distinguishable",
			Body:  "0 done; a Claude Code session may still say within a minute that it held the message, and a task or question taken back that way returns to its sender as a note. 1 the target refused or could not be reached. 2 the call was wrong: unknown command or flag, missing argument, no such session. 3 accepted, not delivered yet: pending lands once the session can take it; held waits for the person at the receiving session to release it or let it expire, and an expired task or question returns to its sender as a note. Branch on the code, not on the text.",
		},
		{
			Title: "Only sessions started through rewake take part",
			Body:  "An agent started by hand is out of reach. Start it with rewake and it appears in list.",
		},
		{
			Title: "A waiting message is announced, not pasted",
			Body:  "A notice is one line, \"Rewake: <session> <kind>, <n> new message(s)\" — with a 🟢 in front where the harness shows plain text, a red circle for an error, yellow for a stopped turn — and a preview of the author's first line, about 100 columns. Ready mail is announced at once, starting or steering a turn, without waiting for a peek or a finished turn. Mail arriving within the first 150 ms joins the notice; later mail goes with the next, never into one already accepted. A notice's members are fixed, each keeping its own sender and obligation, and old unread mail is not announced again. A grant, of a directory or of Git metadata, goes only with the notice of the task or question that carries it. rewake inbox --peek previews, --message <id> reads one, plain inbox reads all. Start every message and final reply with one line stating its point.",
		},
		{
			Title: "Rewake's own lines mostly start with Rewake:",
			Body:  "Notices, send results, inbox headers and notes from rewake open with \"Rewake:\" and say what happened, not how. The exceptions: another session's text, printed as written under \"from <session> · <kind> · <time>\", \"answer from <session>:\" for a question, or \"from <session> · <id> · text no longer kept\" when --owed has lost it; --awaited's \"to <session>\" and \"<id> · <kind> · <time> · <state>\" above each first line; main's state line, \"<session>: <activity> | context … | compactions …\"; and the notices \"Session available.\" and \"Session is no longer available…\", which keep their identity block. A note that a message was not delivered means the agent never saw it: no report is owed and nothing resends it.",
		},
		{
			Title: "Kinds and replies",
			Body:  kindSummary(),
		},
		{
			Title: "Rooms isolate conversations",
			Body:  "--room <name> before the harness name picks the room, default otherwise. Commands inherit REWAKE_ROOM, default without it. Sessions see only their room and names may repeat across rooms: no address or messaging flag reaches another room.",
		},
		{
			Title: "Session addresses come from launch prefixes",
			Body:  "An address is <prefix>-<harness>. The prefix is the role unless --name replaces it, which leaves the role as it is; -<harness> is added even to a prefix that already ends with it. A taken default gets -2, -3 after the harness; a taken --name is refused. A prefix starts with a lower-case letter or digit and holds lower-case letters, digits, dots, dashes and underscores; the address fits 32 characters. Send takes the exact address list shows, and running sessions keep their names.",
		},
		{
			Title: "Choose a session role",
			Body:  roleSummary(),
		},
		{
			Title: "A message from shell cannot be answered with send",
			Body:  "shell means it was typed in a plain terminal, not sent by a session. Answer it in your own reply; rewake cannot deliver to a shell.",
		},
		{
			Title: "Delivery speed differs by harness",
			Body:  "Claude Code receives through its inbox socket; Codex through its own app-server, which starts or steers a turn, in a fresh conversation too. Every send reports acceptance or why delivery failed.",
		},
	}
}
