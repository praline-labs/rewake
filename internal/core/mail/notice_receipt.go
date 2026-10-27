package mail

import "time"

// Receipt is what a harness says about a notice after its delivery attempt
// returned: how a hold ended, or that a notice it was taken to accept was held or
// refused after all. ID is the id of the announcement that was handed over,
// which for a grouped notice names the group rather than a member. At is when
// the harness said it, on the boot clock, zero when unknown: a release is the
// moment the agent has the notice, and the server may hear of it a turn later.
type Receipt struct {
	ID     string
	Result Result
	At     int64
}

// A harness that accepts silently can still speak up late, and a delivery
// counted from its silence is then taken back. So the members of a delivered
// notice are kept this long, and at most this many notices; an adapter that
// sends late words keeps its own record for as long.
const (
	LateWordWindow = time.Minute
	LateWordKeep   = 128
)

// UndeliveredNotice tells the sender of a task or question that its message
// was held and never reached the agent. Nothing is sent again on its behalf.
type UndeliveredNotice struct {
	ID     string `json:"id"`
	Kind   Kind   `json:"kind"`
	Detail string `json:"detail"`
}
