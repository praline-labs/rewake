package cli

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/praline-labs/rewake/internal/host/launch"
	"github.com/praline-labs/rewake/internal/host/spacehost"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// spaceOpened is what rewake space open answers.
type spaceOpened struct {
	Space     string `json:"space"`
	ID        string `json:"id"`
	Session   string `json:"session"`
	Harness   string `json:"harness"`
	Role      string `json:"role"`
	Room      string `json:"room"`
	SessionID string `json:"sessionId,omitempty"`
	Worktree  bool   `json:"worktree,omitempty"`
	// Type is the kind of work the worker was raised for, when it has one.
	Type string `json:"type,omitempty"`
	// Notice is what the worker still lacks that the person gives in its
	// pane — a login of its own —, for main to tell the person.
	Notice string `json:"notice,omitempty"`
}

func (o spaceOpened) lines() []string {
	kind := o.Role
	if o.Type != "" {
		kind += ", type " + o.Type
	}
	lines := []string{
		fmt.Sprintf("Rewake: opened space %s: %s (%s, %s) runs in room %s and closes when this run ends or on rewake space close %s.", o.Space, o.Session, o.Harness, kind, o.Room, o.Space),
		fmt.Sprintf("Give it a task with rewake send %s \"...\"; main in the shell shows its terminal as the worker pane the bar selects.", o.Session),
	}
	if o.Notice != "" {
		lines = append(lines, o.Notice)
	}
	return lines
}

// handleSpaceOpen starts a worker in a space of main's run: it resolves the
// request to a worker (launch.Set.Raise) and opens the space
// (spacehost.Open), then prints what opened.
func handleSpaceOpen(ctx *Context, call Call) error {
	usage := func(format string, args ...any) error {
		return &UsageError{Command: call.Command, Message: fmt.Sprintf(format, args...)}
	}
	if len(call.Positionals) < 2 {
		return usage("space open needs the space's name: rewake space open api --alias <alias>.")
	}
	name := call.Positionals[1]
	dir, err := state.Dir()
	if err != nil {
		return usage("%s", err.Error())
	}
	self, epoch, err := ownRun(dir)
	if errors.Is(err, errNotASession) {
		return usage("only main opens a space, from inside its own session; this shell runs in none.")
	}
	if err != nil {
		return failf("%v", err)
	}
	catalog := ctx.table.catalog
	if err := spacesRefusal(call.Command, catalog, self); err != nil {
		return err
	}
	worker, err := ctx.launchAliases().Raise(catalog, spaceRequest(call))
	if err != nil {
		return usage("%v", err)
	}
	if err := refuseReportingRole(worker.Harness, worker.Role); err != nil {
		return err
	}
	opened, err := spacehost.Open(spacehost.Opening{Dir: dir, Name: name, Main: self.Name, Run: epoch, Worker: worker, Catalog: catalog})
	var wrong *spacehost.RequestError
	if errors.As(err, &wrong) {
		return usage("%v", err)
	} else if err != nil {
		return failf("%v", err)
	}
	answer := spaceOpened{
		Space: name, ID: opened.Record.ID, Session: opened.Session, Harness: worker.Harness.ID(), Role: worker.Role.ID,
		Room: filepath.Base(dir), SessionID: opened.Record.Worker.SessionID, Worktree: worker.Worktree != "", Type: worker.Work.Name,
		Notice: opened.Notice,
	}
	return printValue(ctx, answer, answer.lines)
}

// spaceRequest is what space open asks the worker to be raised with, as
// typed.
func spaceRequest(call Call) launch.Request {
	typed := func(flag string) *string {
		if value, given := call.Flags[flag]; given {
			return &value
		}
		return nil
	}
	return launch.Request{
		Alias:    call.Flags["alias"],
		Worktree: typed("worktree"), NoWorktree: call.Switch("no-worktree"),
		Type: typed("type"), NoType: call.Switch("no-type"),
		Args: call.Positionals[2:],
	}
}
