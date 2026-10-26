package mail

// What the turn-end and read logic takes from whoever runs it: the values the
// core defines for itself, so that no caller's own types reach it.

// TurnEnd is a turn end as the core takes it, whichever way it was reported.
// Whether it ends pending is not part of it: that is decided from the
// session's mark when its reports are prepared.
type TurnEnd struct {
	Boundary *ReadBoundary
	Text     string
	Failed   bool
	Stopped  bool
	// Started and Ended bound the turn on the boot clock, zero where unknown:
	// they decide whether a pending mark was made in this turn.
	Started, Ended int64
	ID             string
}

// Kind names what the end says of the work, which is part of its event and
// so of its operation's name: the strings are kept as they are.
func (e TurnEnd) Kind() string {
	switch {
	case e.Stopped:
		return "stopped"
	case e.Failed:
		return "failed"
	}
	return "finished"
}
