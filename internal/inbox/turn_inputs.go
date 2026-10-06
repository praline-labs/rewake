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
