package control

// Codes an agent names the local check by that refused a command, just
// before its native call: the same on every wire, worded here so a change of
// the words is the wrapper's alone (docs/remote-control.md#the-agents-part).
const (
	// CodeConversation: the command names a conversation the session no
	// longer has selected.
	CodeConversation = "conversation"
	// CodeBusy: another compaction or interrupt is in flight.
	CodeBusy = "busy"
	// CodeTurn: a compaction met a running turn, or an interrupt named a turn
	// that is not the one running.
	CodeTurn = "turn"
	// CodeIdle: an interrupt met no running turn.
	CodeIdle = "idle"
	// CodeClosed: the conversation closed before the native outcome came.
	CodeClosed = "closed"
)

// Refusal words what the agent's code says of action: refused, or for a
// conversation closed under the command an open failure, since the native
// call may have gone out.
func Refusal(action, code string) Answer {
	switch code {
	case CodeConversation:
		return Answer{Outcome: Refused, Reason: NotSelected}
	case CodeBusy:
		return Answer{Outcome: Refused, Reason: Busy}
	case CodeTurn:
		if action == Compact {
			return Answer{Outcome: Refused, Reason: InTurn}
		}
		return Answer{Outcome: Refused, Reason: NoTurn}
	case CodeIdle:
		return Answer{Outcome: Refused, Reason: NoTurn}
	case CodeClosed:
		return Answer{Outcome: Failed, Detail: "the conversation closed before its control outcome", Open: true}
	}
	return Answer{Outcome: Refused, Detail: "unknown control action"}
}
