package gateway

// Selection is a step of the terminal's choice of a conversation, told to
// the mail tool's channel (docs/mail-bridge-channel-codex.md#selecting-a-conversation):
// a selection admitted, its answer, a primary left empty, or another thread
// admitted, which starts that thread's servers too.
type Selection struct {
	Step string
	// Thread is the target an admission names, "" for a thread/start or a
	// fork; the thread an answer selects.
	Thread string
}

// The steps of a selection.
const (
	SelectionAdmitted = "admitted"
	SelectionSelected = "selected"
	SelectionFailed   = "failed"
	ThreadAdmitted    = "thread"
)

// selections tells the steps of the connection that holds the primary; the
// others' states select nothing. It is called with the connection's lock
// held, as ToolEvent is, and never waits.
func (g *Gateway) selections(c *connection) func(Selection) {
	return func(step Selection) {
		if g.cfg.Selection != nil && g.owns(c) {
			g.cfg.Selection(step)
		}
	}
}

// tell passes one step on, where anyone listens.
func (s *state) tell(step, thread string) {
	if s.selection != nil {
		s.selection(Selection{Step: step, Thread: thread})
	}
}

// admit opens a selection: the primary is emptied, as by invalidate, and the
// admission is told in place of a failure.
func (s *state) admit(reason, target string) {
	s.reset(reason)
	s.tell(SelectionAdmitted, target)
}

// selected sets the primary at a selection's successful answer.
func (s *state) selected(thread string) {
	s.Thread, s.Ready = thread, true
	s.tell(SelectionSelected, thread)
}
