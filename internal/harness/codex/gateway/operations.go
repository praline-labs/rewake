package gateway

import (
	"slices"
	"sync"
)

// operations is what the server accepted on each conversation and has not
// been seen to end. The gateway keeps one for all its connections: the server
// keeps an operation it queued when the terminal disconnects, and runs it
// later (docs/remote-control-codex.md), so the doubt it leaves outlives the
// connection it was accepted on. It ends only as it would on one connection:
// by the operation's own end, by the answer to a turn sent after it, or by
// the refusal of a compaction.
type operations struct {
	mu sync.Mutex
	// sent numbers the requests of every connection in the order they are
	// written, so an answer read on one connection orders against an
	// operation accepted on an earlier one.
	sent  uint64
	turns map[string]*threadTurns
	// untracked says an operation was accepted when every one of 64
	// recorded conversations still had one open, and could not be recorded:
	// every conversation stays uncertain for the gateway's life.
	untracked bool
}

// threadTurns is the record of one conversation: the operations the server
// accepted whose end has not been read, and the last turns read to their end.
//
// The server acknowledges a compaction or a review when it has queued it, and
// a turn/start or turn/steer once the session has routed it; either may start
// or end unseen by this connection, and nothing but the turn's own
// turn/completed shows it has ended. While any is open, main's compaction
// cannot tell whether it would abort one, and is refused.
type threadTurns struct {
	// open is keyed by the order the request was sent in (operations.sent).
	// A compaction's turn is "" until its item names it.
	open map[uint64]string
	// overflow is the latest operation past the 64 open that could not be
	// recorded; 0 when none. Only the answer to a later turn ends it.
	overflow uint64
	// ended are the last turns read to their end, so a reply that names one
	// after its turn/completed opens nothing.
	ended []string
}

// uncertainDetail is main's refusal while an accepted operation has not been seen
// to end. It names the ways out that work: the answer to a turn/start, which a
// message delivered or a turn typed at the terminal gets; a goal's turn gets
// none, and does not clear it.
const uncertainDetail = "the gateway cannot tell whether an earlier operation has finished; compact from the TUI, or after the next message delivered to this session or turn typed at the terminal"

// untrackedDetail is main's refusal once an operation could not be recorded,
// which nothing clears.
const untrackedDetail = "the gateway lost count of the operations it accepted, with some open in 64 conversations, and cannot tell whether any has finished until this session's wrapper ends; compact from the TUI"

func newOperations() *operations { return &operations{turns: map[string]*threadTurns{}} }

// next numbers a request in the order it goes to the server. Every request
// that may start a turn is sent while the admission gate is held, so taking
// the number with the gate held is taking the order of the writes.
func (o *operations) next() uint64 {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.sent++
	return o.sent
}

// of is the record of thread, made room for when there is none; nil when
// every recorded conversation still has an operation open. Called under o.mu.
func (o *operations) of(thread string) *threadTurns {
	t := o.turns[thread]
	if t == nil {
		if len(o.turns) >= 64 {
			for id, old := range o.turns {
				if len(old.open) == 0 && old.overflow == 0 {
					delete(o.turns, id)
					break
				}
			}
			if len(o.turns) >= 64 {
				return nil
			}
		}
		t = &threadTurns{open: map[uint64]string{}}
		o.turns[thread] = t
	}
	return t
}

func (o *operations) hasEnded(thread, turn string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	t := o.turns[thread]
	return t != nil && slices.Contains(t.ended, turn)
}

// opened records an operation the server may have accepted.
func (o *operations) opened(thread string, sent uint64, turn string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	t := o.of(thread)
	switch {
	case t == nil:
		o.untracked = true
	case len(t.open) < 64:
		t.open[sent] = turn
	default:
		t.overflow = max(t.overflow, sent)
	}
}

// uncertain says why an operation accepted on thread may not have ended, or
// "" when every one has been seen to.
func (o *operations) uncertain(thread string) string {
	o.mu.Lock()
	defer o.mu.Unlock()
	t := o.turns[thread]
	switch {
	case o.untracked:
		return untrackedDetail
	case t != nil && (len(t.open) > 0 || t.overflow != 0):
		return uncertainDetail
	}
	return ""
}

// answered takes the answer to a turn/start or turn/steer sent as sent: every
// operation sent on the conversation before it has started or been refused by
// then (state.routed), and stops being open.
func (o *operations) answered(thread string, sent uint64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	t := o.turns[thread]
	if t == nil || sent == 0 {
		return
	}
	for id := range t.open {
		if id < sent {
			delete(t.open, id)
		}
	}
	if t.overflow < sent {
		t.overflow = 0
	}
}

// ended takes a turn read to its end: the operation that named it is over.
func (o *operations) ended(thread, turn string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	t := o.of(thread)
	if t == nil {
		return
	}
	for id, named := range t.open {
		if named == turn {
			delete(t.open, id)
		}
	}
	t.ended = append(t.ended, turn)
	if len(t.ended) > 16 {
		t.ended = t.ended[1:]
	}
}

// name sets the turn of the operation sent as sent, when it is still open.
func (o *operations) name(thread string, sent uint64, turn string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if t := o.turns[thread]; t != nil {
		if named, open := t.open[sent]; open && named != turn {
			t.open[sent] = turn
		}
	}
}

// refused drops the operation sent as sent: it cannot start.
func (o *operations) refused(thread string, sent uint64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if t := o.turns[thread]; t != nil {
		delete(t.open, sent)
	}
}

// unanswered records as open every request that may start a turn and was
// written without its answer read, when the connection ends: the server may
// have queued it, and its answer is lost with the connection. A detached
// review's is recorded on the conversation it was asked on, as its thread is
// not known yet. Called under c.mu.
func (c *connection) unanswered() {
	for _, p := range c.state.pending {
		switch p.method {
		case "turn/start", "turn/steer", "review/start":
			if p.target != "" {
				c.state.ops.opened(p.target, p.sent, "")
			}
		}
	}
	for _, p := range c.admitted.pending {
		c.state.ops.opened(p.binding.Thread, p.sent, "")
	}
}
