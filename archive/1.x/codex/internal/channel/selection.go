package channel

import "slices"

// Conversations on Codex (docs/mail-bridge-channel-codex.md): each thread
// starts its own server, so the tool observation counts the connections of
// the conversation the gateway selected — bound to its thread, or not bound
// yet — and moves to another only at the answer that selects it. Between a
// selection's admission and its answer what may be the new conversation's is
// held, and the answer decides whose it was.

// selection is one admitted and not yet answered.
type selection struct {
	at     Stamp
	target string
	// timer is the expected start's end and wall clock, zero when the
	// target's own connection was live at the admission.
	timer int64
	held  []Event
	// base is the conversation's observation when the first of a chain of
	// admissions came, own the events of its own connections counted since,
	// and admissions the chain itself: an answer selecting the conversation
	// again folds all three together by event time from base, so a held
	// hello lands before a later own close as it happened.
	base       observation
	own        []Event
	admissions []Event
}

// observation is the part of the record the timeline folds.
type observation struct {
	tool, class                    string
	classAt, interval, reconnected Stamp
	live                           []uint64
}

func (s *sweep) observation() observation {
	r := s.r
	return observation{r.Tool, r.Class, r.ClassAt, r.Interval, r.Reconnected, slices.Clone(r.Live)}
}

func (s *sweep) restore(o observation) {
	r := s.r
	r.Tool, r.Class, r.ClassAt, r.Interval, r.Reconnected, r.Live = o.tool, o.class, o.classAt, o.interval, o.reconnected, slices.Clone(o.live)
}

// failureKeepsTimer says whether a failure folding now leaves the timer: a
// pending selection's is ended only by its answer, and an own failure of its
// period, refolded at the answer, ended none then.
func (s *sweep) failureKeepsTimer() bool {
	return s.pending != nil || s.owned
}

// ofConversation says whether a connection serves the conversation: bound
// to its thread, or not bound yet. On Claude Code every connection does.
func (s *sweep) ofConversation(g uint64) bool {
	if s.r.Harness != Codex {
		return true
	}
	thread := s.r.h.conns[g].thread
	return thread == "" || thread == s.conv
}

// own says whether a connection is bound to the conversation itself, not
// merely unbound.
func (s *sweep) own(g uint64) bool {
	thread := s.r.h.conns[g].thread
	return s.conv != "" && thread == s.conv
}

// concerns says whether an event is the conversation's: a foreign
// connection's events and a status naming another thread are not. A call's
// failure or ticket is its connection's, so a call admitted for one
// conversation that ends after another is selected proves nothing of the
// new one; one that names no connection is taken by its thread.
func (s *sweep) concerns(e Event) bool {
	switch e.Kind {
	case Hello, Closed, CannotStart, NotObserved, Validated:
		if e.Generation == 0 {
			return e.Thread == "" || e.Thread == s.conv
		}
		return s.ofConversation(e.Generation)
	case StartupFailed:
		return e.Thread == "" || e.Thread == s.conv
	}
	return true
}

// codex applies what is Codex's alone, and says whether the event is done
// with: a selection, an event held while one is pending, a foreign one.
func (s *sweep) codex(e Event) bool {
	switch e.Kind {
	case SelectionAdmitted, Selected, SelectionFailed:
		s.selection(e)
		return true
	}
	if s.pending != nil {
		if s.ownEvent(e) {
			// The old conversation's own connections still count for it,
			// and are kept should the answer select it again.
			s.pending.own = append(s.pending.own, e)
			return false
		}
		if e.Kind != ThreadAdmitted {
			s.pending.held = append(s.pending.held, e)
		}
		return true
	}
	return !s.concerns(e)
}

// ownEvent says whether an event is of a connection bound to the
// conversation, which a pending selection does not hold.
func (s *sweep) ownEvent(e Event) bool {
	switch e.Kind {
	case Hello, Closed, CannotStart, NotObserved, Validated:
		return e.Generation != 0 && s.own(e.Generation)
	}
	return false
}

// selection applies a selection event in the timeline.
func (s *sweep) selection(e Event) {
	switch e.Kind {
	case SelectionAdmitted:
		s.admit(e)
	case Selected:
		if s.pending == nil {
			// An answer with no admission before it opens nothing and
			// holds nothing.
			s.pending = &selection{at: e.At, target: e.Thread, base: s.observation()}
		}
		s.selected(e)
	case SelectionFailed:
		if s.pending != nil {
			// Dropped with what it held: nothing is told, and the display
			// stays the old conversation's.
			s.pending = nil
			s.r.Timer = 0
		}
	}
}

// admit opens a pending selection. Its timer opens at the admission
// unless it names its target and a connection bound to that thread was live
// then; one admitted while another is pending takes its place and keeps
// what that one held.
func (s *sweep) admit(e Event) {
	next := &selection{at: e.At, target: e.Thread, base: s.observation()}
	if s.pending != nil {
		next.held, next.own, next.base = s.pending.held, s.pending.own, s.pending.base
		next.admissions = slices.Clone(s.pending.admissions)
	}
	next.admissions = append(next.admissions, e)
	s.pending = next
	s.expect(e)
	next.timer = s.r.Timer
}

// expect opens the timer of an admission, or stops the one running when it
// names a thread one of whose connections is live.
func (s *sweep) expect(e Event) {
	if len(s.targetLive(e.Thread, e.At.Boot)) > 0 {
		s.r.Timer = 0
		return
	}
	s.open(e.At, HelloAtStart)
}

// targetLive are the connections bound to a target thread live at a time.
func (s *sweep) targetLive(target string, at int64) []uint64 {
	var live []uint64
	if target == "" {
		return nil
	}
	for g, c := range s.r.h.conns {
		if c.thread == target && c.liveAt(at) {
			live = append(live, g)
		}
	}
	return live
}

// selected applies the answer selecting thread B. The conversation already:
// its own state goes on, and what it counted while the selection was pending
// folds again with what was held and the admissions, together by event time
// from where it stood at the first admission — a held hello before an own
// close shows the close was no failure. Once answered, the start the
// admissions expected is the conversation's, so a hello or a ticket of its
// own ends that wait at its own time, as a held one does; an own failure
// leaves the timer as it did while pending. Another: the old
// one's connections are foreign from now on and its interval closes untold;
// the new one starts from its connections live at the admission, connected
// with one and starting with none, never working on the old one's proof,
// and what was held that is its own folds at its own time.
func (s *sweep) selected(e Event) {
	r, pending := s.r, s.pending
	s.pending = nil
	replay := make([]refold, 0, len(pending.held))
	for _, e := range pending.held {
		replay = append(replay, refold{e, false})
	}
	if e.Thread == s.conv {
		s.restore(pending.base)
		for _, e := range slices.Concat(pending.admissions, pending.own) {
			replay = append(replay, refold{e, true})
		}
		slices.SortFunc(replay, func(a, b refold) int { return order(a.Event, b.Event) })
	} else {
		s.conv = e.Thread
		r.Class, r.ClassAt, r.Interval, r.Reconnected = "", Stamp{}, Stamp{}, Stamp{}
		r.Live = s.targetLive(e.Thread, pending.at.Boot)
		slices.Sort(r.Live)
		r.Tool = ToolStarting
		if len(r.Live) > 0 {
			r.Tool = ToolConnected
		}
		r.Timer = pending.timer
	}
	for _, f := range replay {
		s.pass(f.At.Boot)
		switch {
		case f.Kind == SelectionAdmitted:
			s.expect(f.Event)
		case f.own:
			s.owned = true
			s.apply(f.Event)
			s.owned = false
		case s.concerns(f.Event):
			s.apply(f.Event)
		}
	}
}

// refold is an event an answer folds again, and whether the conversation
// counted it as its own while the selection was pending.
type refold struct {
	Event
	own bool
}
