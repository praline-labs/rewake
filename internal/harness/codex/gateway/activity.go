package gateway

import (
	"encoding/json"
	"time"
)

// activity is what the selected conversation is running, as far as this
// connection has read it: whether a turn runs, and which. The turn an
// interrupt names and the "a turn is running" refusal of a compaction read it.
//
// It goes with the selection: invalidate resets it, because after the
// terminal leaves a conversation the server stops sending it that
// conversation's events, and a turn id read before may name a turn long ended
// when it comes back. The resume's reply says what runs then. Which frame sets
// and clears it, and the server's order each step relies on, is in
// docs/remote-control-codex.md.
type activity struct {
	// turn is the running turn's id; "" while a turn runs whose id has not
	// been seen, as after an active status or a resume without turns.
	turn    string
	running bool
	// announced says the server has said the turn runs — turn/started, an
	// active status or a resume's snapshot — not only named it in a reply.
	announced bool
}

// replied updates the record from a reply of the server. Called under c.mu
// after state.response, with the request the reply answers: admission is set
// for a turn/start or turn/steer the gate admitted, the terminal's or a
// delivery's; requested for any request of the terminal's.
func (c *connection) replied(m meta, raw []byte, p pending, requested bool, admission admittedRequest, admitted bool) {
	s := &c.state
	if m.failure {
		return
	}
	defer c.marked(admission.binding.Thread)
	defer c.marked(p.target)
	switch {
	case admitted:
		s.routed(admission.binding.Thread, admission.sent, m.turn, admission.binding.Generation == s.Generation)
	case requested && (p.method == "turn/start" || p.method == "turn/steer"):
		c.admitted.worked(p.target, m.turn)
		s.routed(p.target, p.sent, m.turn, p.generation == s.Generation)
	case requested && p.method == "review/start":
		// An inline review runs on the conversation; a detached one on a
		// conversation of its own, which the reply names, and whose turn it
		// shows to be work all the same. Its reply says only that it is
		// queued, so it proves nothing about earlier work.
		if review := str(raw, "result", "reviewThreadId"); review == "" || review == p.target {
			c.admitted.worked(p.target, m.turn)
			s.accept(p.target, p.sent, m.turn, p.generation == s.Generation)
		} else {
			c.admitted.worked(review, m.turn)
		}
	case requested && p.intent && s.Ready && p.generation == s.Generation && m.thread == s.Thread:
		s.resumed(m.status, raw)
		c.resumedMark(m.thread)
	}
}

// resumedMark ends the hold of a tied mark when the reply to a selection shows
// its compaction no longer running: the terminal was away when its turn ended,
// so no turn/completed of it will come, and the hold would last to its bound.
// Only the hold ends — the reply says nothing of how the compaction ended, so
// main's wait goes on — and a reply that shows nothing either way keeps it. A
// delivery sent into a compaction running after all is refused by the server
// and waits (ErrCompacting). Called under c.mu after state.resumed.
func (c *connection) resumedMark(thread string) {
	marker := c.admitted.manual[thread]
	d := c.state.doing
	if marker == nil || marker.turn == "" || d.running && (d.turn == "" || d.turn == marker.turn) {
		return
	}
	marker.released = true
}

// routed takes the reply to a turn/start or turn/steer. The session handles
// its submissions one at a time and answers this one only after routing it,
// so every operation sent before it on the conversation has started or been
// refused by then, and cannot first start later: those stop being open.
func (s *state) routed(thread string, sent uint64, turn string, current bool) {
	s.ops.answered(thread, sent)
	s.accept(thread, sent, turn, current)
}

// accept takes a turn a reply names. The reply and the turn's events are
// sent from different tasks in the server, so the turn may have been read to
// its end already, and then nothing runs. Only a reply to a request of the
// current selection names the running turn; one of an earlier selection may
// name a turn long ended, and is only open.
func (s *state) accept(thread string, sent uint64, turn string, current bool) {
	if thread == "" || turn == "" || s.ops.hasEnded(thread, turn) {
		return
	}
	s.ops.opened(thread, sent, turn)
	if current && s.Ready && thread == s.Thread && (!s.doing.running || s.doing.turn != turn) {
		s.doing = activity{turn: turn, running: true}
	}
}

// resumed takes what the reply to a selection says runs: the snapshot's last
// turn in progress, for which no turn/started follows. It ends nothing open:
// work the server has queued and not yet taken is not in the snapshot.
func (s *state) resumed(status string, raw []byte) {
	running := runningTurn(raw)
	switch {
	case status == "active":
		s.doing = activity{turn: running, running: true, announced: true}
	case running != "":
		s.doing = activity{turn: running, running: true, announced: true}
	default:
		s.doing = activity{}
	}
}

// runningTurn is the id of the last turn in progress in a selection's reply,
// or "" when the reply carries no turns.
func runningTurn(raw []byte) string {
	var turns []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if json.Unmarshal(field(raw, "result", "thread", "turns"), &turns) != nil {
		return ""
	}
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].Status == "inProgress" {
			return turns[i].ID
		}
	}
	return ""
}

// terminal says a turn/completed ends its turn: the server sends it with one
// of these statuses when the turn's task is done, and a turn has no other end
// this connection can read.
func terminal(m meta) bool {
	return m.method == "turn/completed" && m.turn != "" && (m.status == "completed" || m.status == "failed" || m.status == "interrupted")
}

// announced updates the record from an event, after admittedWork.event has
// tied a compaction mark to its turn. Called under c.mu.
func (c *connection) announced(m meta) {
	s := &c.state
	if m.thread == "" {
		return
	}
	c.marked(m.thread)
	if terminal(m) {
		s.ops.ended(m.thread, m.turn)
	}
	if !s.Ready || m.thread != s.Thread {
		return
	}
	d := &s.doing
	switch {
	case m.method == "turn/started":
		*d = activity{turn: m.turn, running: true, announced: true}
	case m.method == "thread/status/changed" && m.status == "active":
		d.running, d.announced = true, true
	case m.method == "thread/status/changed" && m.status == "idle":
		// A run whose turn is unknown is over when the conversation goes
		// idle; a known turn waits for its turn/completed, which the
		// server sends after the status.
		if d.turn == "" {
			*d = activity{}
		}
	case terminal(m):
		if d.turn == m.turn || d.turn == "" {
			*d = activity{}
		}
	case m.turn != "" && d.running && d.turn == "" && !s.ops.hasEnded(m.thread, m.turn):
		// After a resume without turns the running turn's id is unknown
		// until an event of it carries one.
		d.turn = m.turn
	}
}

// marked follows the compaction mark of thread into the record: the open
// operation of the compaction names the mark's turn, and names no turn again
// when the mark is untied from a turn shown to be work. Called under c.mu.
func (c *connection) marked(thread string) {
	marker := c.admitted.manual[thread]
	if thread == "" || marker == nil {
		return
	}
	c.state.ops.name(thread, marker.sent, marker.turn)
}

// dropStale ends the hold of every compaction mark past its bound: the start
// bound while its turn has not been seen, the running bound once it has
// (steer.go). Deliveries go again, and the wait for its end ends: with the
// reason when nothing was seen to start, and as still running otherwise, whose
// end is then main's outcome when it is seen. The mark itself stays until its
// turn's end, so a compaction that starts late is still counted as asked for.
// Called under c.mu.
func (c *connection) dropStale(now time.Time) {
	start, run := c.owner.markLimit(), c.owner.runLimit()
	for _, marker := range c.admitted.manual {
		switch {
		case marker.turn == "" && !marker.released && now.Sub(marker.asked) >= start:
			marker.released = true
			marker.finish("", "the compaction's turn was not seen to start within "+start.String()+"; deliveries go on, and its turn is still not taken for work if it starts")
		case marker.turn != "" && now.Sub(marker.asked) >= run:
			// A resume may have ended the hold before, and not the wait.
			marker.released = true
			if marker.ended != nil && !marker.answered {
				marker.stillRunning("the compaction was not seen to end within " + run.String())
			}
		}
	}
}

// lostDetail is main's letter's detail when the terminal left the conversation before
// the compaction's turn was seen.
const lostDetail = "the gateway lost sight of the compaction (the terminal left the conversation before it started)"

// lostSight gives up every compaction mark not yet tied whose selection has
// changed since its request: the server sends a conversation's events only
// while it is selected, so the compaction's item may have passed unseen, and
// a turn seen after the gap is not known to be it. The mark never ties;
// the wait for its end ends, deliveries go on, and the compaction's operation stays
// open. Called under c.mu after anything that may change the selection.
func (c *connection) lostSight() {
	for _, marker := range c.admitted.manual {
		if marker.turn != "" || marker.lost || marker.generation == c.state.Generation {
			continue
		}
		marker.lost, marker.released = true, true
		marker.finish("", lostDetail)
	}
}
