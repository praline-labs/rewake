package cli

import (
	"github.com/praline-labs/rewake/internal/inbox"
)

// turnResult is a turn end as the core takes it, whichever way the harness
// reported it. Whether it ends pending is not part of it: that is decided
// from the session's mark when its reports are prepared.
type turnResult struct {
	Boundary *inbox.ReadBoundary
	Text     string
	Failed   bool
	Stopped  bool
	// Started and Ended bound the turn on the boot clock, zero where unknown:
	// they decide whether a pending mark was made in this turn.
	Started, Ended int64
	ID             string
}

// kind names what the end says of the work, which is part of its event.
func (r turnResult) kind() string {
	switch {
	case r.Stopped:
		return "stopped"
	case r.Failed:
		return "failed"
	}
	return "finished"
}
