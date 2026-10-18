package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/praline-labs/rewake/internal/core/space"
	"github.com/praline-labs/rewake/internal/host/spacehost"
	"github.com/praline-labs/rewake/internal/host/terminal"
	"github.com/praline-labs/rewake/internal/infra/state"
)

func attachCommand() *Command {
	return &Command{
		Name:           "attach",
		Args:           "<space>",
		MaxPositionals: 1,
		Summary:        "Connect this terminal to a space's worker, to answer what it asks; " + spacehost.DetachKeyName + " detaches and leaves it running.",
		Examples:       []string{"rewake attach api", "rewake attach api-claude"},
		Next:           []string{"rewake space list"},
		Notes: []string{
			"For the person, from a terminal of their own: the worker's terminal takes this one's place, in raw mode, its size following this window's; every key goes to the worker but " + spacehost.DetachKeyName + ", which detaches. On attach the worker is resized, so a full-screen harness repaints.",
			"A space is named by its name or its worker's session, <name>-<harness>, in the room of this shell as for every command. One terminal is attached at a time.",
			"Exit 0 detached or the worker ended; 1 the space is not open or its host does not answer, or another terminal is attached; 2 a wrong call — no such space, or this is no terminal.",
		},
		Access:  viewForm,
		Handler: handleAttach,
	}
}

func handleAttach(ctx *Context, call Call) error {
	if len(call.Positionals) != 1 {
		return &UsageError{Command: call.Command, Message: "attach needs the space: rewake attach api."}
	}
	if !terminal.IsTerminal(os.Stdin) || !terminal.IsTerminal(os.Stdout) {
		return &UsageError{Command: call.Command, Message: "attach connects a terminal, and this one's input or output is not one: run it in the person's own terminal."}
	}
	dir, err := state.Dir()
	if err != nil {
		return &UsageError{Command: call.Command, Message: err.Error()}
	}
	record, err := findSpace(dir, call.Positionals[0])
	if err != nil {
		return &UsageError{Command: call.Command, Message: err.Error()}
	}
	if record.State != space.Open {
		return failf("the space %s is %s, and only an open space has a terminal to attach to.", record.Name, record.State)
	}
	session := record.Worker.Name + "-" + record.Worker.Harness
	_, _ = fmt.Fprintf(ctx.Stderr, "Rewake: attached to %s (%s); %s detaches and leaves it running.\r\n", record.Name, session, spacehost.DetachKeyName)
	detached, err := spacehost.Attach(state.SpaceSocketPath(dir, record.ID), os.Stdin, os.Stdout)
	if errors.Is(err, spacehost.ErrBusy) {
		return failf("%v", err)
	}
	if err != nil {
		return failf("could not attach to %s: %v", record.Name, err)
	}
	if detached {
		_, _ = fmt.Fprintf(ctx.Stderr, "\r\nRewake: detached from %s; it keeps running.\n", record.Name)
		return nil
	}
	_, _ = fmt.Fprintf(ctx.Stderr, "\r\nRewake: the terminal of %s closed: its worker ended.\n", record.Name)
	return nil
}

// findSpace is the space a word names that is not over: by its name, or by
// its worker's session.
func findSpace(dir, word string) (space.Record, error) {
	records, _, err := space.List(dir)
	if err != nil {
		return space.Record{}, err
	}
	var last *space.Record
	for i := range records {
		r := records[i]
		if r.Name != word && r.Worker.Name+"-"+r.Worker.Harness != word {
			continue
		}
		if !r.State.Final() {
			return r, nil
		}
		last = &records[i]
	}
	if last != nil {
		return *last, nil
	}
	return space.Record{}, fmt.Errorf("no space %s in this room; rewake space list shows them", word)
}
