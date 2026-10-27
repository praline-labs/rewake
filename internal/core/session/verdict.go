package session

// Verdict is what a snapshot says a session does, judged in one place for
// every reader: `rewake list` and the letters' headers, main's report to
// herdr and the shell's strip and button render it and derive nothing
// (docs/session-state.md#activity). It is judged when the snapshot is read,
// never stored: whether it is stale is the reader's to know.
type Verdict struct {
	// Activity is the word for what the session does, one of the Activity
	// values.
	Activity string `json:"activity"`
	// Label is Activity as a person reads it: with "; interruptions
	// unheard" where the session says so, and " (stale)" when Stale.
	Label string `json:"label"`
	// Blocked is what the session waits on, one of the Blocked values; empty
	// when it waits on nothing.
	Blocked string `json:"blocked"`
	// Stale says an observed activity is not current: the run stopped
	// publishing, or no longer observes it.
	Stale bool `json:"stale"`
	// Mark is the button's edge in the shell, one of the Mark values.
	Mark string `json:"mark"`
}

// Values of Verdict.Activity.
const (
	ActivityIdle       = "idle"
	ActivityWorking    = "working"
	ActivityCompacting = "compacting"
	ActivityPermission = "waiting for a permission"
	ActivityQuestion   = "waiting for an answer"
	ActivityDialog     = "waiting at a dialog"
	ActivityUnknown    = "unknown"
)

// Values of Verdict.Blocked.
const (
	BlockedPermission = WaitingPermission
	BlockedQuestion   = WaitingQuestion
	BlockedDialog     = "dialog"
)

// Values of Verdict.Mark: off when nothing goes on, steady while the session
// works, blinking when it stopped for a reason (docs/shell.md, "The bar").
const (
	MarkOff    = "off"
	MarkSteady = "steady"
	MarkBlink  = "blink"
)

// Judge is the verdict on the snapshot as it stands: call it after whatever
// marks the snapshot stale.
func (s Snapshot) Judge() Verdict {
	v := Verdict{Activity: ActivityUnknown}
	if s.Activity != nil && (*s.Activity == ActivityIdle || *s.Activity == ActivityWorking) {
		v.Activity = *s.Activity
		// A compaction is what the session does while it lasts.
		if s.Compacting != nil && *s.Compacting {
			v.Activity = ActivityCompacting
		}
		// A dialog stops the session whatever its turn is doing: one a
		// request carries to main says what it asks, any other main was told
		// of.
		switch {
		case s.Waiting == WaitingPermission:
			v.Activity, v.Blocked = ActivityPermission, BlockedPermission
		case s.Waiting == WaitingQuestion:
			v.Activity, v.Blocked = ActivityQuestion, BlockedQuestion
		case s.Dialog != nil:
			v.Activity, v.Blocked = ActivityDialog, BlockedDialog
		}
	}
	// One freshness rule for every reader: the activity is current only
	// while the snapshot is (Fresh) and its own observation is
	// (ActivityFresh). The paths that mark a snapshot old — a lost link, a
	// stopped publisher, a replaced run, a departure — mark one or both, so
	// a reader asking for both depends on none of them. Only an observed
	// activity can be stale; one never reported is unknown.
	v.Stale = s.Activity != nil && (!s.ActivityFresh || !s.Fresh)
	v.Label = v.Activity
	// Only the gap is named. A session that hears interruptions behaves as
	// expected and needs no word in a narrow cell, and a harness that always
	// hears them never sets the field.
	if s.Interruptions == InterruptionsUnobserved {
		v.Label += "; interruptions unheard"
	}
	if v.Stale {
		v.Label += " (stale)"
	}
	switch {
	case v.Blocked != "":
		v.Mark = MarkBlink
	case v.Activity == ActivityWorking || v.Activity == ActivityCompacting:
		// A turn whose snapshot went stale has stopped somewhere nobody sees.
		v.Mark = MarkSteady
		if v.Stale {
			v.Mark = MarkBlink
		}
	default:
		v.Mark = MarkOff
	}
	return v
}
