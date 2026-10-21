package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/space"
	"github.com/praline-labs/rewake/internal/host/spacehost"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// closeBound is how long rewake space close waits for the space to be over:
// the space host reads the record once a second, and its close sequence takes
// at most eight seconds before it records the end.
const closeBound = 20 * time.Second

// closedByMain is why a space main closed began to close; the space host's
// end adds the signal that ended the worker.
const closedByMain = "main closed it with rewake space close"

// spaceClosed is what rewake space close answers.
type spaceClosed struct {
	Space   string      `json:"space"`
	ID      string      `json:"id"`
	Session string      `json:"session"`
	State   space.State `json:"state"`
	Why     string      `json:"why"`
	// Checkout is the worker's checkout, kept when the space is: it stays
	// listed by rewake space list until it is gone.
	Checkout string `json:"checkout,omitempty"`
	Branch   string `json:"branch,omitempty"`
	// Stays is why the close did not remove the checkout: something in it
	// would have been lost.
	Stays string `json:"stays,omitempty"`
}

func (c spaceClosed) lines() []string {
	lines := []string{fmt.Sprintf("Rewake: closed space %s: %s ended (%s).", c.Space, c.Session, c.Why)}
	if c.State == space.Kept {
		stays := ""
		if c.Stays != "" {
			stays = " because " + c.Stays
		}
		lines = append(lines, fmt.Sprintf("Its checkout %s%s stays%s: rewake space list shows what is in it and how to clear it.", c.Checkout, keptOn(space.Reminder{Branch: c.Branch}), stays))
	}
	return lines
}

// handleSpaceClose ends the worker of a space main's run opened: it moves the
// record to closing with main's reason, and the space host, which reads the
// record once a second, ends the worker as it does at main's end and records
// what it observed (docs/rules/space.md, SP10). Main signals nothing itself.
func handleSpaceClose(ctx *Context, call Call) error {
	usage := func(format string, args ...any) error {
		return &UsageError{Command: call.Command, Message: fmt.Sprintf(format, args...)}
	}
	if len(call.Positionals) != 2 {
		return usage("space close takes the space's name and nothing else: rewake space close api.")
	}
	for _, flag := range []string{"alias", "worktree"} {
		if _, given := call.Flags[flag]; given {
			return usage("--%s belongs to space open.", flag)
		}
	}
	name := call.Positionals[1]
	dir, err := state.Dir()
	if err != nil {
		return usage("%s", err.Error())
	}
	self, epoch, err := ownRun(dir)
	if errors.Is(err, errNotASession) {
		return usage("only main closes a space, from inside its own session; this shell runs in none.")
	}
	if err != nil {
		return failf("%v", err)
	}
	if !role.Of(self.Role).Spaces {
		return usage("%s is a %s session, and only main closes a space.", self.Name, role.Of(self.Role).ID)
	}
	records, problems, err := space.List(dir)
	if err != nil {
		return failf("could not read the spaces in %s: %v", dir, err)
	}
	var record space.Record
	var open []string
	for _, candidate := range records {
		if candidate.State.Final() {
			continue
		}
		open = append(open, candidate.Name)
		if candidate.Name == name {
			record = candidate
		}
	}
	switch {
	case record.ID == "" && len(problems) > 0:
		return failf("cannot tell whether a space %s is open: %s: %s", name, problems[0].Path, problems[0].Why)
	case record.ID == "" && len(open) == 0:
		return usage("no space named %s is open; this room has no open space.", name)
	case record.ID == "":
		return usage("no space named %s is open; open spaces: %s.", name, strings.Join(open, ", "))
	case record.Main.Run != epoch:
		return failf("the space %s belongs to another run of main, %s; it closes when that run ends.", name, record.Main.Run)
	case record.State == space.Opening:
		return failf("the space %s is still opening; close it once rewake space open has answered.", name)
	case record.State == space.Open:
		if _, err := space.BeginClosing(dir, record.ID, closedByMain); err != nil {
			return failf("could not close the space %s: %v", name, err)
		}
	}
	// A space already closing is waited for as it is: its host is ending it.
	// One whose host is proven gone has nobody to end it, so the close does,
	// by the same sequence.
	if space.HostRuns(record) == proc.IdentityEnded {
		current, err := space.Load(dir, record.ID)
		if err != nil {
			return failf("could not read the record of space %s: %v", name, err)
		}
		if current.State == space.Closing {
			if err := spacehost.FinishGone(dir, current); err != nil {
				return failf("the space host of %s is gone and the close could not finish it: %v", name, err)
			}
		}
	}
	over, err := awaitOver(dir, record)
	if err != nil {
		return err
	}
	why, stays := over.End.Reasons()
	closed := spaceClosed{Space: name, ID: over.ID, Session: name + "-" + over.Worker.Harness, State: over.State, Why: why}
	if over.State == space.Kept {
		closed.Checkout, closed.Branch, closed.Stays = over.Checkout.Path, over.Checkout.Branch, stays
	}
	return printValue(ctx, closed, closed.lines)
}

// awaitOver waits until the space host recorded the space's end.
func awaitOver(dir string, record space.Record) (space.Record, error) {
	deadline := time.Now().Add(closeBound)
	for {
		current, err := space.Load(dir, record.ID)
		if err != nil {
			return space.Record{}, failf("could not read the record of space %s: %v", record.Name, err)
		}
		if current.State.Final() {
			return current, nil
		}
		if time.Now().After(deadline) {
			return space.Record{}, failf("the space %s is still %s after %s: its space host has not recorded the worker's end; rewake space list shows it.", record.Name, current.State, closeBound)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
