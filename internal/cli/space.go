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

// The spaces of a room (docs/v2/stage4-space.md#9-commands): a worker a
// main's run hosts in a pseudo-terminal of its own, in a checkout of its
// own, and the record that stays after it. list is the view; it reads the
// records and looks at the kept checkouts, and writes nothing.

func spaceGroup() Group {
	return Group{
		Title:   "SPACES",
		Summary: "A space is a worker a main's run hosts in a terminal of its own, in a checkout of its own; its record stays after it closes.",
		Commands: []*Command{{
			Name:           "space",
			Args:           "list",
			MaxPositionals: 1,
			Summary:        "List this room's spaces, open and closed, and the checkouts closed spaces kept.",
			Options:        []Option{jsonOption},
			Examples:       []string{"rewake space list", "rewake space list --json"},
			Next:           []string{"rewake list"},
			Notes: []string{
				"Each space shows its main, its worker's name, harness and role, the session id rewake chose for the worker's conversation (none for a harness that takes none), its checkout and branch. A closed space keeps them.",
				"States: opening — the record is written, the worker not yet seated; open — the worker runs in its checkout; closing; closed — over, no checkout left; kept — over, its checkout left. A space not over whose main has ended reads \"main ended\"; one whose main cannot be told from here reads \"main unknown\".",
				"A kept checkout is listed with what was found in it — changes, commits, a Git lock its keeper left, or that it could not be looked at — and the Git commands that clear it once what is wanted is merged; each refuses rather than lose work. It stays listed until the checkout is gone, and one that cannot be looked at stays too. rewake removes none.",
				"Exit 0 listed; 1 the spaces could not be read.",
			},
			Access:  viewForm,
			Handler: handleSpace,
		}},
	}
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
	if verb != "list" {
		return &UsageError{Command: call.Command, Message: fmt.Sprintf("space needs list, got %q: rewake space list.", verb)}
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
