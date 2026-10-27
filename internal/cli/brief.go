package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/taskbrief"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// briefGroup is the brief command: main writes a markdown text per task and
// sends it with send --brief; the runs it was sent to tick its items and
// comment (docs/briefs.md).
func briefGroup() Group {
	return Group{
		Title:   "BRIEFS",
		Summary: "A brief is a markdown text for one task, kept in the room: main writes and edits it and gives it with send --brief; the runs it was sent to read it with their task, tick its items and comment.",
		Commands: []*Command{{
			Name:           "brief",
			Args:           "new <text> | edit <id> --rev <n> <text> | close <id> | list | show <id> | tick <id> <n> --edition <e> | comment <id> -- <text>",
			MaxPositionals: Variadic,
			Summary:        "Make, edit, close and list briefs from main; read, tick and comment one from a run it was sent to.",
			Options: []Option{
				{Flag: "--file", Value: "<path>", Summary: "new, edit: read the text from this file instead of the argument."},
				{Flag: "--dir", Value: "<path>", Summary: "new: put the brief in <path>/brief-<id> instead of where the project's [briefs] dir or the room keeps it; the directory must exist."},
				{Flag: "--rev", Value: "<n>", Summary: "edit, required: the revision the new text starts from, as brief show printed it."},
				{Flag: "--edition", Value: "<e>", Summary: "tick, required: the edition the item's number was read in, as brief show printed it."},
				{Flag: "--undo", Summary: "tick: mark the item not done."},
				{Flag: "--closed", Summary: "list: the closed briefs too, after the open ones."},
				{Flag: "--markdown", Summary: "show: only the text with the ticks applied and no numbers, to edit it."},
				jsonOption,
			},
			Examples: []string{
				"rewake brief new \"# Auth refactor\n\n- [ ] move the login\n- [ ] run the tests\"",
				"rewake brief new --file .scratch/auth.md --json",
				"rewake brief new --dir .shift/tasks/auth --file .scratch/auth.md",
				"rewake brief show 3", "rewake brief show 3 --markdown", "rewake brief show 3 --json",
				"rewake brief edit 3 --rev 7 --file .scratch/auth.md",
				"rewake brief list", "rewake brief list --closed --json",
				"rewake brief tick 3 2 --edition 1", "rewake brief tick 3 2 --edition 1 --undo",
				"rewake brief comment 3 -- \"the login moved; the tests wait on the fixture\"",
				"rewake brief close 3",
			},
			Next: []string{"rewake send <name> \"text\" --brief <id>"},
			Notes: []string{
				"new, edit, close and list are main's. show is main's and a run's the brief was sent to with send --brief; tick and comment are only such a run's, while the brief is open. A run's right ends when it ends or the brief closes; a later run of the same name has none. Anyone else is refused.",
				"A brief's id is a number from 1. Its title is its first \"# \" heading, else its first line. Its items are its task-list lines, \"- [ ] ...\", numbered from 1 in the text's order; show prints each number after its box.",
				"Two numbers stay apart: the edition counts main's edits, the revision is the log's latest edit, tick or untick. A tick names the edition its number was read in and is refused once main edited since: read the brief again. An edit names the revision it started from and is refused once anything changed since: edit the text show --markdown prints, which carries the ticks as [x].",
				"A text is at most 64 KiB, a comment at most 1000 characters; neither may be empty. A comment that begins with a dash goes after --. Use - as the text to read it from stdin.",
				"An edit tells nobody: send a letter with --brief for a worker to read the new text. A closed brief is archived whole; brief list --closed shows it. A brief in a project's folder is marked closed where it is.",
				"Where a brief goes: --dir, else the project's .rewake/aliases.toml [briefs] dir = \"<path>\" (relative to main's directory), else the room's state, which a reboot may clear. Ids stay per room; a brief whose folder was moved lists as unreadable \"moved\" and rewake brief close marks it closed.",
				"Exit 0 done; 1 refused — not main or not a run the brief was sent to, a closed brief, a stale edition or revision; 2 a wrong call — no such brief or item, a missing or bad argument.",
			},
			Access:  briefAccess,
			Handler: handleBrief,
		}},
	}
}

// briefAccess classifies brief by its verb: new, edit, close, tick and
// comment write the brief's records once they name what they act on; list,
// show, and a verb the handler refuses, only answer.
func briefAccess(call Call) access {
	if len(call.Positionals) > 0 {
		switch call.Positionals[0] {
		case "new", "edit", "close", "tick", "comment":
			return accessChange
		}
	}
	return accessView
}

// briefFlags are the flags each verb takes; any other is refused rather than
// run as if it were not there.
var briefFlags = map[string][]string{
	"new":     {"file", "dir"},
	"edit":    {"file", "rev"},
	"close":   {},
	"list":    {"closed"},
	"show":    {"markdown"},
	"tick":    {"edition", "undo"},
	"comment": {},
}

func handleBrief(ctx *Context, call Call) error {
	verb := ""
	if len(call.Positionals) > 0 {
		verb = call.Positionals[0]
	}
	allowed, known := briefFlags[verb]
	if !known {
		return &UsageError{Command: call.Command, Message: fmt.Sprintf("brief needs new, edit, close, list, show, tick or comment, got %q: rewake brief list, or rewake brief show 3.", verb)}
	}
	for flag := range call.Flags {
		if flag != "json" && !slices.Contains(allowed, flag) {
			return &UsageError{Command: call.Command, Message: fmt.Sprintf("brief %s does not take --%s.", verb, flag)}
		}
	}
	dir, err := state.Dir()
	if err != nil {
		return &UsageError{Command: call.Command, Message: err.Error()}
	}
	caller, err := briefCaller(ctx, dir)
	if err != nil {
		return err
	}
	home, homeErr := briefHomeOf(dir)
	if homeErr != nil && verb == "new" {
		return failf("%v", homeErr)
	}
	args := call.Positionals[1:]
	switch verb {
	case "new":
		return briefNew(ctx, call, home, caller, args)
	case "edit":
		return briefEdit(ctx, call, home, caller, args)
	case "close":
		return briefClose(ctx, call, home, caller, args)
	case "list":
		return briefList(ctx, call, home, caller, args)
	case "show":
		return briefShow(ctx, call, home, caller, args)
	case "tick":
		return briefTick(ctx, call, home, caller, args)
	}
	return briefComment(ctx, call, home, caller, args)
}

// briefRun is the run that calls a brief command, as its environment names
// it and the registry confirms.
type briefRun struct {
	session registry.Session
	author  taskbrief.Author
}

func (r briefRun) isMain() bool { return r.session.Role == role.Main.ID }

func briefCaller(ctx *Context, dir string) (briefRun, error) {
	self, epoch, err := ownRunIn(ctx.access, dir)
	if err != nil {
		return briefRun{}, failf("a brief is read and changed only by main and the runs it was sent to, and this shell is not one: %v. The person reads briefs in rewake's shell.", err)
	}
	return briefRun{session: self, author: taskbrief.Author{Name: self.Name, Epoch: epoch}}, nil
}

// mainOnly refuses a verb of main's to any other run.
func (r briefRun) mainOnly(verb string) error {
	if r.isMain() {
		return nil
	}
	return failf("only main may %s a brief; %s is a %s session. Ask main to do it.", verb, r.session.Name, r.session.Role)
}

// briefID reads the id a verb acts on, the first of its arguments.
func briefID(call Call, verb string, args []string) (int, error) {
	if len(args) == 0 {
		return 0, &UsageError{Command: call.Command, Message: fmt.Sprintf("brief %s needs the brief's id: rewake brief %s 3.", verb, verb)}
	}
	id, ok := taskbrief.ParseID(args[0])
	if !ok {
		return 0, &UsageError{Command: call.Command, Message: fmt.Sprintf("%q is not a brief's id: a number from 1, as rewake brief list shows.", args[0])}
	}
	return id, nil
}

// briefText is the text of new or edit: the argument, - for stdin, or
// --file, exactly one of them.
func briefText(call Call, verb string, args []string) (string, error) {
	file, fromFile := call.Flags["file"]
	switch {
	case fromFile && len(args) > 0:
		return "", &UsageError{Command: call.Command, Message: fmt.Sprintf("brief %s takes the text or --file, not both.", verb)}
	case fromFile:
		opened, err := os.Open(file)
		if err != nil {
			return "", &UsageError{Command: call.Command, Message: fmt.Sprintf("could not read %s: %v", file, err)}
		}
		defer func() { _ = opened.Close() }()
		raw, err := readBounded(opened)
		if err != nil {
			return "", &UsageError{Command: call.Command, Message: fmt.Sprintf("could not read %s: %v", file, err)}
		}
		return raw, nil
	case len(args) == 0:
		return "", &UsageError{Command: call.Command, Message: fmt.Sprintf("brief %s needs the text, - for stdin, or --file <path>.", verb)}
	case len(args) > 1:
		return "", &UsageError{Command: call.Command, Message: "Quote the text as one argument: loose words are refused, not joined."}
	case args[0] == "-":
		raw, err := readBounded(os.Stdin)
		if err != nil {
			return "", failf("could not read the text from stdin: %v", err)
		}
		return raw, nil
	}
	return args[0], nil
}

// readBounded reads a text up to a brief's bound and one byte more, so a
// larger one is refused for its size without reading it all.
func readBounded(from io.Reader) (string, error) {
	raw, err := io.ReadAll(io.LimitReader(from, taskbrief.MaxText+1))
	return string(raw), err
}

// briefError gives a refusal of the brief's records the CLI's shape.
func briefError(call Call, err error) error {
	var (
		moved    *taskbrief.MovedError
		closed   *taskbrief.ClosedError
		unbound  *taskbrief.NotBoundError
		revision *taskbrief.StaleRevisionError
		edition  *taskbrief.StaleEditionError
		item     *taskbrief.NoItemError
	)
	switch {
	case errors.Is(err, taskbrief.ErrNotFound):
		return &UsageError{Command: call.Command, Message: "no such brief: rewake brief list shows the open ones, and --closed the closed ones too."}
	case errors.Is(err, taskbrief.ErrBadText), errors.As(err, &item):
		return &UsageError{Command: call.Command, Message: err.Error() + "."}
	case errors.As(err, &moved):
		return failf("%v.", err)
	case errors.As(err, &closed), errors.As(err, &unbound), errors.As(err, &revision), errors.As(err, &edition):
		return failf("%v.", err)
	}
	return failf("%v", err)
}
