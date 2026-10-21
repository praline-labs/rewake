package spacehost

import (
	"errors"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/space"
	"github.com/praline-labs/rewake/internal/infra/proc"
)

// goneHost is added to a space's reason when main's close ended it without
// the space host, which was no longer there to.
const goneHost = "; its space host was gone"

// FinishGone ends a space whose host is provably gone — killed, or crashed —
// as the host would have: what runs of the worker's process group is stopped
// by the close sequence, the checkout is released by today's rule and the
// record is closed from what was seen. The space must be closing already (its
// reason is the record's). A worker that cannot be told alive or ended, or a
// group still there after SIGKILL, leaves it closing for the next try.
func FinishGone(dir string, record space.Record) error {
	if record.State != space.Closing {
		return errors.New("only a space that is closing is finished without its host")
	}
	pid, _, _, ok := registry.ParseRun(record.Worker.Run)
	if !ok {
		return errors.New("the space stays closing: the worker's run does not read, so its end cannot be shown")
	}
	why := record.Closing + goneHost
	// The wrapper's end does not end its group: a harness it started may run
	// on, and the checkout is released only once none of the group is left.
	observed := registry.ObserveRun(record.Worker.Run)
	if observed == proc.IdentityUnknown {
		return errors.New("the space stays closing: the worker's run cannot be told alive or ended from here")
	}
	if proc.GroupAlive(pid) && (observed != proc.IdentityEnded || !pidReused(pid)) {
		// Not this process's child, so there is nothing to reap: the group
		// is gone when nothing but zombies is left in it.
		reaped := make(chan struct{})
		close(reaped)
		switch took := stopGroup(pid, reaped, stopping); took {
		case "":
			return errors.New("the space stays closing: the worker's process group was still there after SIGKILL")
		case "nothing":
		default:
			why += "; the worker's process group ended after " + took
		}
	}
	// Enter: whether the worker was allowed a checkout is in the host's spec,
	// which went with it, so the record is asked once more whether the
	// worker's session claimed one.
	h := &host{spec: Spec{Dir: dir, ID: record.ID, Enter: true}, record: record}
	return h.close(why)
}

// pidReused says a worker wrapper read ended has its pid held by another live
// process: a group of that number is then not the worker's, since a group's
// number is not handed out while any member is left.
func pidReused(pid int) bool {
	state, err := proc.State(pid)
	return err == nil && state != "Z"
}
