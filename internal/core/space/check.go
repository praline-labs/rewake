package space

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// check holds what every record says of itself, whatever step wrote it: the
// worker fixed before it starts, the space host recorded before the worker
// runs, an end exactly when the space is over and one that observed
// something.
// A write that breaks it is refused, and a record read that breaks it is
// unreadable rather than half believed.
func (r Record) check() error {
	problem := func(format string, args ...any) error {
		return fmt.Errorf("the record of space %q: %s", r.Name, fmt.Sprintf(format, args...))
	}
	switch {
	case !ValidID(r.ID):
		return problem("%q is no space id", r.ID)
	case !state.ValidName(r.Name):
		return problem("the name is unusable")
	case r.State.rank() == 0:
		return problem("%q is no state", r.State)
	case r.OpenedAt.IsZero():
		return problem("no time of opening")
	case !state.ValidName(r.Main.Name) || r.Main.Run == "":
		return problem("no main run")
	case r.Worker.Harness == "" || !state.ValidName(r.Worker.Name) || r.Worker.Role == "":
		return problem("the worker's harness, name and role are fixed before it starts")
	case r.Worker.SessionID != "" && !sessionIDShape.MatchString(r.Worker.SessionID):
		return problem("%q is no session id rewake draws", r.Worker.SessionID)
	case r.Checkout.Path != "" && !filepath.IsAbs(r.Checkout.Path):
		return problem("the checkout %q is not absolute", r.Checkout.Path)
	case r.Checkout.Path == "" && r.Checkout.Branch != "":
		return problem("a branch without its checkout")
	case r.State == Open && (r.Host == "" || r.Worker.Run == ""):
		return problem("open without its space host or its worker's run")
	case r.State == Closing && r.Closing == "":
		return problem("closing without why")
	case r.State.Final() != (r.End != nil):
		return problem("an end is set exactly when the space is over")
	}
	if r.End == nil {
		return nil
	}
	if err := r.End.check(r); err != nil {
		return problem("%v", err)
	}
	return nil
}

func (e End) check(r Record) error {
	switch {
	case e.At.IsZero() || e.Why == "":
		return errors.New("an end says when and why")
	case e.Worker == WorkerEnded && r.Worker.Run == "":
		return errors.New("a worker seen to end had a run")
	case e.Worker == WorkerNeverStarted && r.Worker.Run != "":
		return errors.New("a worker with a run did start")
	case e.Worker != WorkerEnded && e.Worker != WorkerNeverStarted:
		return fmt.Errorf("%q is no observed end of a worker", e.Worker)
	case e.Checkout.Presence == "":
		return errors.New("an end says how the checkout was found: there, gone or unknown")
	case e.Checkout.Presence != There && e.Checkout.Presence != Gone && e.Checkout.Presence != Unknown:
		return fmt.Errorf("%q is no presence of a checkout", e.Checkout.Presence)
	case e.Checkout.Presence != Gone && r.Checkout.Path == "":
		return errors.New("a checkout found there or unknown that the record never named")
	case e.Checkout.Presence != There && (e.Checkout.Changes || e.Checkout.Commits || e.Checkout.Locked):
		return errors.New("what was found in a checkout not seen there")
	case (r.State == Kept) != (e.Checkout.Presence != Gone):
		return errors.New("kept exactly while the checkout was not seen gone at the close")
	}
	return nil
}
