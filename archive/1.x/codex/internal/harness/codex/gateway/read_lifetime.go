package gateway

// These are admitted work/control operations, not evidence of a different target.
// Revocation is conservative for injected work: it need not end the TUI's borrow.
func endsReadWorkflow(method string) bool {
	switch method {
	case "turn/start", "turn/steer", "turn/interrupt", "review/start", "thread/compact/start",
		"thread/goal/set", "thread/goal/clear", "thread/archive", "thread/delete", "thread/unarchive":
		return true
	}
	return false
}

// The ordinary goal query follows awaited backfill in the pinned resume path.
// Native rename/title application also runs through the App command/event loop.
// Injected metadata materialization does not have either implication.
func nativeReadBoundary(m meta) bool {
	return endsReadWorkflow(m.method) || m.method == "thread/goal/get" && m.numeric || m.method == "thread/name/set" && !isHelper(m)
}

func (s *state) closeReadContext(reason string) {
	s.readSerial++
	s.canBackfill = false
	s.backfill = nil
	s.readClosedBy = reason
}

func (s *state) readPhase() string {
	if s.canBackfill {
		return "armed"
	}
	for _, p := range s.pending {
		if p.backfill && p.readSerial == s.readSerial && p.generation == s.Generation {
			return "list-pending"
		}
	}
	if len(s.backfill) > 0 {
		return "cohort"
	}
	return "closed"
}

func (s *state) readScope(thread string) bool {
	if thread == "" || thread == s.Thread {
		return true
	}
	for _, p := range s.pending {
		if p.intent && p.generation == s.Generation && p.target == thread {
			return true
		}
	}
	return false
}

// A visible auxiliary control for the selected target cannot leave its old read
// exception armed. Unrelated helper work does not identify a primary workflow.
func (g *Gateway) auxiliaryReadBoundary(origin *connection, m meta) {
	if !endsReadWorkflow(m.method) {
		return
	}
	c := g.currentConnection()
	if c == nil || c == origin {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state.readScope(m.thread) {
		c.state.closeReadContext("auxiliary " + m.method)
		c.record("auxiliary-work-boundary", m)
	}
}
