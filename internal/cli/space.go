package cli

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/space"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The spaces of a room (docs/rules/space.md): a worker main starts in a
// terminal of its own, in main's room, living no longer than main's run, and
// the record that stays after it. list is the view; open is main's change;
// attach connects the person's terminal to a worker's.

func spaceGroup() Group {
	return Group{
		Title:   "SPACES",
		Summary: "A space is a worker main starts with rewake space open: an ordinary session of main's room in a terminal of its own, which ends when main's run ends; its record stays after it closes.",
		Commands: []*Command{{
			Name:           "space",
			Args:           "list | open <name> [harness args...]",
			MaxPositionals: Variadic,
			Summary:        "List this room's spaces, or, from main, open one: start a worker that lives no longer than main's run.",
			Options: []Option{
				{Flag: "--harness", Value: "<harness>", Summary: "open: the worker's harness, by its launch command's name."},
				{Flag: "--role", Value: "<role>", Summary: "open: the worker's role, " + role.Write.ID + " by default; " + role.General.ID + " works too."},
				{Flag: "--worktree", Summary: "open: start the worker in a new worktree, as the harness's own --worktree launch does; --worktree=<name> names it."},
				jsonOption,
			},
			Examples: []string{
				"rewake space list", "rewake space list --json",
				"rewake space open api --harness claude",
				"rewake space open api --harness claude --worktree=fix-login -- --permission-mode acceptEdits",
				"rewake space open docs --harness claude --role general --json",
			},
			Next: []string{"rewake send <name>-<harness> \"text\"", "rewake attach <name>"},
			Notes: []string{
				"open is main's: refused from any other role, and from a run whose spaces are not live. The worker is an ordinary launch of rewake in main's room, named <name>-<harness>, started in main's working directory; everything after the name goes to its harness — permission flags, a model. rewake types nothing into the worker on its own: what it needs to start unattended goes there; the person types into it from main's Workers pane (/workers) or with rewake attach. rewake chooses the session id of a harness that takes one and keeps it in the record.",
				"The worker runs in a pseudo-terminal of a space host, a rewake process detached from main and no descendant of it. The space host checks main's run once a second; once that run has ended — by /exit, a crash or a kill — it sends the worker's process group SIGHUP, then SIGTERM after a second, then SIGKILL after five more, records what it observed, and exits.",
				"open answers once the worker's session is registered, within 30 seconds, or says why the space did not open — a worker that exited at once is named with the last line it wrote.",
				"Each space shows its main, its worker's name, harness and role, the session id rewake chose for the worker's conversation (none for a harness that takes none), its checkout and branch. A closed space keeps them.",
				"States: opening — the record is written, the worker not yet started; open — the worker runs; closing; closed — over, no checkout left; kept — over, its checkout left. A space not over whose main has ended reads \"main ended\"; one whose main cannot be told from here reads \"main unknown\".",
				"A kept checkout is listed with what was found in it — changes, commits, a Git lock, or that it could not be looked at — and the Git commands that clear it once what is wanted is merged; each refuses rather than lose work. It stays listed until the checkout is gone, and one that cannot be looked at stays too. rewake removes none.",
				"Exit 0 listed or opened; 1 the spaces could not be read, or the space did not open; 2 a wrong call — not main, no such harness or role, a name that cannot be one.",
			},
			Access:  spaceAccess,
			Handler: handleSpace,
		}, attachCommand()},
	}
}

// spaceAccess classifies space by its verb: open writes the record and starts
// the space host; list, and a verb the handler refuses, only answer.
// spaceAccess makes open a change once it names a space; a bare open is
// refused by the handler before anything is read.
func spaceAccess(call Call) access {
	if len(call.Positionals) > 1 && call.Positionals[0] == "open" {
		return accessChange
	}
	return accessView
}

// spaceView is one space as list reports it.
type spaceView struct {
	space.Record
	// MainRun is whether the main of a space not over still runs: running,
	// ended or unknown; empty once the space is over.
	MainRun string `json:"mainRun,omitempty"`
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
	case verb != "list":
		return &UsageError{Command: call.Command, Message: fmt.Sprintf("space needs list or open, got %q: rewake space list, or rewake space open api --harness claude.", verb)}
	case len(call.Positionals) > 1:
		return &UsageError{Command: call.Command, Message: "space list takes no other argument."}
	}
	for _, flag := range []string{"harness", "role", "worktree"} {
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
	lines = append(lines, fmt.Sprintf("Rewake: %d kept %s until removed; rewake removes none. Merge what is wanted, then clear each from its repository:", len(l.Kept), checkouts))
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
