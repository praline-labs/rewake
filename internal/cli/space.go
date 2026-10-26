package cli

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/praline-labs/rewake/internal/core/space"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The spaces of a room (docs/rules/space.md): a worker main starts in a
// terminal of its own, in main's room, living no longer than main's run, and
// the record that stays after it. list is the view; open and close are
// main's changes.
// The person sees a worker's terminal in the shell (docs/shell.md).

func spaceGroup() Group {
	return Group{
		Title:   "SPACES",
		Summary: "A space is a worker main starts with rewake space open: an ordinary session of main's room in a terminal of its own, which ends when main closes it or main's run ends; its record stays after it closes.",
		Commands: []*Command{workersCommand(), {
			Name:           "space",
			Args:           "list | open <name> --alias <alias> [options] [-- <harness args...>] | close <name>",
			MaxPositionals: Variadic,
			Summary:        "List this room's spaces, or, from main, open one — start a worker that lives no longer than main's run — or close one.",
			Options: []Option{
				{Flag: "--alias", Value: "<alias>", Summary: "open, required: the worker's template from rewake workers; supplies its harness, role, wrapper and arguments."},
				{Flag: "--worktree", Summary: "open: start the worker in a new worktree, as the harness's own --worktree launch does; --worktree=<name> names it. Wins over the template's worktree = true. A .worktreeinclude at the repository's top names the ignored files, a .env or a local configuration, copied into the new checkout (rewake worktree --help)."},
				{Flag: "--no-worktree", Summary: "open: start the worker without a worktree although its template has worktree = true."},
				{Flag: "--type", Value: "<type>", Summary: "open: the kind of work the worker is raised for, a type of rewake workers, whose text follows its role's briefing. Wins over the template's type."},
				{Flag: "--no-type", Summary: "open: raise the worker with no type although its template names one."},
				jsonOption,
			},
			Examples: []string{
				"rewake space list", "rewake space list --json",
				"rewake space open api --alias claude-write",
				"rewake space open api --alias claude-write -- --effort low",
				"rewake space open study --alias claude-general --type analysis",
				"rewake space open api --alias claude-write --worktree=fix-login -- --permission-mode acceptEdits",
				"rewake space open docs --alias claude-general --json",
				"rewake space close api", "rewake space close api --json",
			},
			Next: []string{"rewake send <name>-<harness> \"text\""},
			Notes: []string{
				"A worker is raised only from a template: --alias starts the worker an alias of rewake workers describes — its harness, role, command and arguments, with its model and effort, and its context where the harness has a flag for it. The space names the worker and its room. Harness flags after -- go after the template's arguments, and each the harness takes only once replaces the template's copy, an applied model, effort or context included.",
				"A type is the kind of work — " + builtInTypes() + ", or one of the person's or the project's own: --type names it, else the template's type = \"<type>\", else none; --no-type leaves the template's out. Its text, resolved here in main's directory, follows the worker's role briefing; it changes how the worker works, never what it may do. A type that is not there, or a built-in one an alias file's [types] off turned off, is refused with where.",
				"open is main's: refused from any other role, and from a run whose spaces are not live. The worker is an ordinary launch of rewake in main's room whatever its checkout, named <name>-<harness>, started in main's working directory; the only way to start one, since a person's rewake <harness> is always main. Put the harness's flags — permission flags, a model — after --; the options before it belong to space open. rewake types nothing into the worker on its own: what it needs to start unattended goes there; the person types into it from main's shell, where main runs whenever it was started on a terminal, which shows each worker's terminal beside main's. rewake chooses the session id of a harness that takes one and keeps it in the record.",
				"The worker runs in a pseudo-terminal of a space host, a rewake process detached from main and no descendant of it. The space host checks main's run once a second; once that run has ended — by /exit, a crash or a kill — it sends the worker's process group SIGHUP, then SIGTERM after a second, then SIGKILL after five more, records what it observed, and exits.",
				"open answers once the worker's session is registered, within 30 seconds, or says why the space did not open — a worker that exited at once is named with the last line it wrote. A worker that needs what only the person gives in its pane opens all the same, and the answer says what — a Claude Code worker whose own configuration directory has no login yet: the person types /login once in that worker's pane. A pi worker shares the login of ~/.config/rewake/pi/agent with a pi main and opens with no such word: where pi was never logged in there, the person types /login once in its pane.",
				"close is main's, for a space its own run opened: a worker is never switched or restarted in place; when another is needed, close it and open a new one from a template. close records the space closing with main's reason; its space host, within a second, ends the worker by the same sequence as at main's end and records what it observed. close answers once the space is over, within 20 seconds: closed, or kept with its checkout, which space list then shows. A task the worker still owed reads as lost in rewake inbox --awaited. A space whose host process is gone (killed, crashed) is ended by close itself, by the same sequence, its reason saying the host was gone.",
				"Each space shows its main, its worker's name, harness and role, the session id rewake chose for the worker's conversation (none for a harness that takes none), its checkout and branch. A closed space keeps them.",
				"States: opening — the record is written, the worker not yet started; open — the worker runs; closing; closed — over, no checkout left; kept — over, its checkout left. A space not over whose main has ended reads \"main ended\"; one whose main cannot be told from here reads \"main unknown\".",
				"A kept checkout is listed with what was found in it — changes, commits, a Git lock, or that it could not be looked at —, why the close kept it, and the Git commands that clear it once what is wanted is merged; each refuses rather than lose work. It stays listed until the checkout is gone, and one that cannot be looked at stays too. A close removes the checkout and its branch itself only when nothing would be lost: no changes, no edits hidden by an assume-unchanged or skip-worktree mark, no lock or submodule, and every commit on another local branch; files git ignores go with it, which rewake worktree rm would keep.",
				"Exit 0 listed, opened or closed; 1 the spaces could not be read, the space did not open, or did not close within the bound, or belongs to another run of main; 2 a wrong call — not main, no --alias or no such alias, no such type or one turned off, a name that cannot be one, no open space of that name.",
			},
			Access:  spaceAccess,
			Handler: handleSpace,
		}},
	}
}

// spaceAccess classifies space by its verb: open writes the record and starts
// the space host, close moves the record to closing; list, and a verb the
// handler refuses, only answer. Open and close are changes once they name a
// space; a bare one is refused by the handler before anything is read.
func spaceAccess(call Call) access {
	if len(call.Positionals) > 1 && (call.Positionals[0] == "open" || call.Positionals[0] == "close") {
		return accessChange
	}
	return accessView
}

// openOnlyFlags are the space flags only open takes; list and close refuse
// them rather than run as if they were not there.
var openOnlyFlags = []string{"alias", "worktree", "no-worktree", "type", "no-type"}

// spaceView is one space as list reports it.
type spaceView struct {
	space.Record
	// MainRun is whether the main of a space not over still runs: running,
	// ended or unknown; empty once the space is over.
	MainRun string `json:"mainRun,omitempty"`
	// Socket is where the space's host serves its worker's screen, for a
	// space not over: the shell connects there and builds no path itself
	// (docs/shell.md).
	Socket string `json:"socket,omitempty"`
}

type spaceListing struct {
	Room   string      `json:"room"`
	Spaces []spaceView `json:"spaces"`
	// Kept are the checkouts closed spaces left, while they are there.
	Kept []space.Reminder `json:"kept"`
	// Unreadable are the files of the spaces directory that do not read as
	// a record; a space among them is neither listed nor taken for absent.
	Unreadable []space.Problem `json:"unreadable"`
}

func handleSpace(ctx *Context, call Call) error {
	verb := ""
	if len(call.Positionals) > 0 {
		verb = call.Positionals[0]
	}
	switch {
	case verb == "open":
		return handleSpaceOpen(ctx, call)
	case verb == "close":
		return handleSpaceClose(ctx, call)
	case verb != "list":
		return &UsageError{Command: call.Command, Message: fmt.Sprintf("space needs list, open or close, got %q: rewake space list, rewake space open api --alias <alias>, or rewake space close api.", verb)}
	case len(call.Positionals) > 1:
		return &UsageError{Command: call.Command, Message: "space list takes no other argument."}
	}
	for _, flag := range openOnlyFlags {
		if _, given := call.Flags[flag]; given {
			return &UsageError{Command: call.Command, Message: fmt.Sprintf("--%s belongs to space open.", flag)}
		}
	}
	dir, err := state.Dir()
	if err != nil {
		return &UsageError{Command: call.Command, Message: err.Error()}
	}
	records, problems, err := space.List(dir)
	if err != nil {
		return failf("could not read the spaces in %s: %v", dir, err)
	}
	listing := spaceListing{Room: filepath.Base(dir), Spaces: []spaceView{}, Kept: space.Reminders(records), Unreadable: problems}
	if listing.Kept == nil {
		listing.Kept = []space.Reminder{}
	}
	if listing.Unreadable == nil {
		listing.Unreadable = []space.Problem{}
	}
	for _, record := range records {
		view := spaceView{Record: record}
		if !record.State.Final() {
			view.MainRun = map[proc.IdentityEvidence]string{proc.IdentityAlive: "running", proc.IdentityEnded: "ended"}[space.MainRuns(record)]
			if view.MainRun == "" {
				view.MainRun = "unknown"
			}
			view.Socket = state.SpaceSocketPath(dir, record.ID)
		}
		listing.Spaces = append(listing.Spaces, view)
	}
	return printValue(ctx, listing, listing.lines)
}

func (l spaceListing) lines() []string {
	var lines []string
	if len(l.Spaces) == 0 {
		lines = append(lines, "No spaces in room "+l.Room+".")
	} else {
		var buffer bytes.Buffer
		writer := tabwriter.NewWriter(&buffer, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(writer, "SPACE\tSTATE\tMAIN\tWORKER\tHARNESS\tROLE\tSESSION ID\tBRANCH\tCHECKOUT")
		for _, view := range l.Spaces {
			_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", view.Name, view.stateLabel(), view.Main.Name, view.Worker.Name, view.Worker.Harness, view.Worker.Role,
				orNone(view.Worker.SessionID), orDash(view.Checkout.Branch), orDash(view.Checkout.Path))
		}
		_ = writer.Flush()
		lines = append(lines, "room: "+l.Room)
		lines = append(lines, strings.Split(strings.TrimRight(buffer.String(), "\n"), "\n")...)
	}
	for _, problem := range l.Unreadable {
		lines = append(lines, "unreadable: "+problem.Path+": "+problem.Why)
	}
	if len(l.Kept) == 0 {
		return lines
	}
	checkouts := "checkout stays"
	if len(l.Kept) > 1 {
		checkouts = "checkouts stay"
	}
	lines = append(lines, fmt.Sprintf("Rewake: %d kept %s until removed: each held what its close would have lost. Merge what is wanted, then clear each from its repository:", len(l.Kept), checkouts))
	for _, kept := range l.Kept {
		lines = append(lines, "  "+kept.Space+": "+kept.Path+keptOn(kept)+": "+foundLabel(kept))
		for _, command := range kept.Clear {
			lines = append(lines, "    "+command)
		}
	}
	return lines
}

// stateLabel is the state, and for a space not over a main that is not seen
// running.
func (v spaceView) stateLabel() string {
	if v.MainRun == "ended" || v.MainRun == "unknown" {
		return string(v.State) + " (main " + v.MainRun + ")"
	}
	return string(v.State)
}

func keptOn(kept space.Reminder) string {
	if kept.Branch == "" {
		return ""
	}
	return " on " + kept.Branch
}

// foundLabel says what was found in a kept checkout, and whether it can be
// looked at now.
func foundLabel(kept space.Reminder) string {
	var found []string
	for _, item := range []struct {
		set  bool
		word string
	}{{kept.Found.Changes, "changes"}, {kept.Found.Commits, "commits"}, {kept.Found.Locked, "locked"}} {
		if item.set {
			found = append(found, item.word)
		}
	}
	label := "nothing found in it at the close"
	switch {
	case kept.Found.Presence == space.Unknown:
		label = "it could not be looked at at the close"
	case len(found) > 0:
		label = strings.Join(found, ", ")
	}
	if kept.Stays != "" {
		label += "; kept because " + kept.Stays
	}
	if kept.Presence == space.Unknown {
		label += "; it cannot be looked at now"
	}
	return label
}

func orNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
