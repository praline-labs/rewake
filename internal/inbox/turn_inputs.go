package inbox

// What the turn-end and read logic takes from whoever runs it: the values the
// core defines for itself, so that no caller's own types reach it.

// AttemptScope says where an attempt of an operation runs relative to the
// turn the operation was made in
// (docs/mail-bridge-turns.md#a-pending-mark-at-its-turns-end). A pending
// mark speaks for that turn only, and only an attempt shown to run in it
// writes one.
type AttemptScope struct {
	// First: the attempt that created the operation's record, or a call
	// with no record at all, runs in the operation's own turn whatever the
	// transport.
	First bool
	// InOwnTurn: a later attempt is shown to run in that same turn, by a
	// transport whose turn ids are never reused.
	InOwnTurn bool
}

// ReadEvidence is what was observed of one tool result reaching the model,
// the evidence a read part needs before it counts as shown.
type ReadEvidence struct {
	// CallID is the native call the result answered.
	CallID string
	// AnswerDigest names the text the result carried, by the digest the
	// call recorded with each part before printing it; empty when the
	// result carried no text to name.
	AnswerDigest string
	// Whole says the model got the entire result: anything less
	// acknowledges nothing.
	Whole bool
}

// EndGate orders a read's acknowledgment against the turn ends the process
// running the session captures
// (docs/mail-bridge-turns.md#a-turns-end-meets-its-calls). The acknowledgment
// calls Enter holding the mailbox lock, before its first write: false says an
// end at or after calledBoot was noted, and nothing may be written. Otherwise
// it is registered as writing, and calls leave once, whatever its writes did,
// before it releases the mailbox lock.
type EndGate interface {
	Enter(calledBoot int64) (leave func(), ok bool)
}

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
