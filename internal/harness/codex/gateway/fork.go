package gateway

import "time"

// A fork response creates a child; it does not by itself select that child.
// CLI intent comes from launch arguments. In-session lineage replacement also
// requires the native TUI's successful detach of its previously accepted parent.
type forkSelection struct {
	oldSide                              string
	parentLive, detaching, sidePreparing bool
	generation                           uint64
	parent, child, status                string
	startup                              bool
}

func forkIntent(m meta) bool {
	// The terminal's fork carries its configuration as its start and resume do
	// (tuiConfig); read in the source of 0.157.1, not seen live.
	// legacy(codex <0.157.1): the roots or the permissions mark 0.155.1's fork; remove when 0.155.1 is no longer supported
	return m.method == "thread/fork" && m.numeric && m.source == "user" && m.thread != "" && m.config && (m.roots || m.permissions || m.tuiConfig)
}

func (g *Gateway) startupForkIntent(c *connection, m meta) bool {
	if !forkIntent(m) {
		return false
	}
	c.mu.Lock()
	initialized := c.state.initialized
	serial := c.state.Connection
	c.mu.Unlock()
	if !initialized {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.cfg.StartupFork || g.startupBound || g.closed || len(g.owners) > 1 || g.current != nil && g.current != c {
		return false
	}
	if g.startupForkOwner != 0 && (g.startupForkOwner != serial || g.startupForkParent != m.thread) {
		return false
	}
	g.startupForkOwner = serial
	g.startupForkParent = m.thread
	return true
}

// Only the bounded fork workflow may preserve the candidate. Other selection,
// control or side preparation revokes it; later detaches cannot revive a child.
func (s *state) forkRequest(m meta, p *pending) bool {
	if forkIntent(m) && (m.startupFork || s.Ready && m.thread == s.Thread) {
		oldSide := s.side
		s.admit("fork selection pending its native acknowledgements", "")
		s.fork = &forkSelection{generation: s.Generation, parent: m.thread, startup: m.startupFork, oldSide: oldSide, parentLive: true}
		p.fork = true
		p.generation = s.Generation
		return true
	}
	f := s.fork
	if f == nil {
		return false
	}
	if m.method == "thread/unsubscribe" && m.numeric && m.thread == f.parent && f.child != "" {
		if f.sidePreparing {
			s.invalidate("conflicting fork and side workflow; select /resume or /new")
			return true
		}
		f.detaching = true
		p.forkDetach = true
		p.generation = s.Generation
		return true
	}
	if isHelper(m) {
		return false
	}
	if m.method == "thread/inject_items" {
		if m.numeric && m.thread == f.child && f.child != "" && !f.startup && !f.detaching && !f.sidePreparing && f.parentLive {
			f.sidePreparing = true
			p.sideSetup = true
			p.generation = s.Generation
		} else {
			s.invalidate("unproved side preparation; select /resume or /new")
		}
		return true
	}
	if m.thread == f.oldSide && f.oldSide != "" {
		switch m.method {
		case "turn/interrupt", "thread/unsubscribe":
			return true
		}
	}

	if m.thread == f.child && f.child != "" {
		switch m.method {
		case "thread/read", "thread/turns/list", "thread/items/list", "thread/name/set":
			return true
		}
	}
	if uuidID(m.idText) {
		switch m.method {
		case "thread/list", "thread/loaded/list", "thread/read", "thread/turns/list", "thread/items/list":
			return false
		}
	}
	// Non-thread inventory requests do not advance or authorize selection.
	switch m.method {
	case "account/read", "model/list", "configRequirements/read", "collaborationMode/list", "hooks/list", "skills/list", "plugin/list":
		return false
	}
	s.invalidate("fork workflow changed before selection was proved; select /resume or /new")
	return false
}

func (s *state) forkResponse(m meta, p pending) {
	f := s.fork
	if f == nil || p.generation != s.Generation || f.generation != s.Generation {
		return
	}
	if m.failure {
		s.invalidate("fork workflow refused; select /resume or /new")
		return
	}
	if p.sideSetup {
		if !m.resultObject || !f.parentLive {
			s.invalidate("side setup did not preserve a confirmed primary; select /resume or /new")
			return
		}
		s.selected(f.parent)
		s.Reason = "side confirmed; primary unchanged"
		s.side = f.child
		s.events.bind(f.parent, "idle", time.Now())
		s.fork = nil
		return
	}
	if p.fork {
		if m.thread == "" || m.thread == f.parent || !m.directKnown || !m.direct {
			s.invalidate("fork child is not confirmed for direct input; select /resume or /new")
			return
		}
		f.child, f.status = m.thread, m.status
		if f.startup {
			s.acceptFork()
		}
		return
	}
	if detached(m) {
		s.acceptFork()
	} else {
		s.invalidate("fork detach acknowledgement is unknown; select /resume or /new")
	}
}

func (s *state) acceptFork() {
	f := s.fork
	s.selected(f.child)
	s.Reason = "accepted native fork selection"
	s.events.bind(f.child, f.status, time.Now())
	s.fork = nil
}

func detached(m meta) bool {
	if m.failure {
		return false
	}
	switch m.detachStatus {
	case "unsubscribed", "notSubscribed", "notLoaded":
		return true
	}
	return false
}

// Side activity never selects the delivery destination. Its scoped requests are
// allowed only for the one child proved by fork plus side preparation ACKs.
func (s *state) sideRequest(m meta, p *pending) bool {
	if s.side == "" || m.thread != s.side || !m.numeric {
		return false
	}
	switch m.method {
	case "turn/start", "turn/steer", "turn/interrupt", "thread/read", "thread/turns/list", "thread/items/list", "thread/name/set", "thread/settings/update", "thread/goal/get", "thread/goal/set", "thread/goal/clear", "thread/compact/start":
		return true
	case "thread/unsubscribe":
		p.sideDetach = true
		p.generation = s.Generation
		return true
	}
	return false
}

func (s *state) closedThread(thread string) {
	if thread == s.side {
		s.side = ""
	}
	if s.fork != nil && thread == s.fork.parent {
		s.fork.parentLive = false
		if s.fork.sidePreparing {
			s.invalidate("primary closed during side preparation; select /resume or /new")
			return
		}
	}
	if thread == s.Thread || s.fork != nil && thread == s.fork.child {
		s.invalidate("thread closed; explicitly resume primary")
	}
}
