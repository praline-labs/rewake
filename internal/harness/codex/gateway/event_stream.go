package gateway

import (
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/sessionstate"
)

func (c *connection) readServer() {
	defer c.close()
	for {
		raw, err := c.up.readMessage()
		if err != nil {
			c.closeWith("server-to-tui", "read", err, 0)
			return
		}
		var receivedRead uint64
		if c.owner.cfg.ReadSequence != nil {
			receivedRead = c.owner.cfg.ReadSequence()
		}
		m, err := project(raw)
		if err != nil {
			c.closeWith("server-to-tui", "projection", err, len(raw))
			return
		}
		var warn *sessionstate.DeliveryHold
		var warnAt Binding
		if m.id == "" && (m.method == "turn/completed" || m.method == "thread/status/changed" && (m.status == "idle" || m.status == "systemError")) && c.owner.cfg.ReadSequence != nil {
			if capture := c.owner.cfg.EndCapture; capture != nil && c.primary(m.thread) {
				receivedRead, m.endedAt = capture()
			}
			m.readThrough = &receivedRead
		}
		c.mu.Lock()
		observationPending, observationCorrelated := c.state.pending[m.id]
		if m.method == "" {
			admission, admitted := c.admitted.pending[m.id]
			c.admitted.ack(m)
			if waiter, ok := c.injected[m.id]; ok {
				c.replied(m, raw, pending{}, false, admission, admitted)
				delete(c.injected, m.id)
				if environments := field(raw, "result", "thread", "environments"); len(environments) <= 64<<10 {
					m.environments = append([]byte(nil), environments...)
				}
				if reason := field(raw, "error", "message"); len(reason) <= 4096 {
					m.refusal = decodeText(reason)
				}
				waiter <- m
				c.record("injected-reply", m)
				out := c.collectOutcomes()
				c.mu.Unlock()
				c.complete(out)
				continue
			}
			if strings.HasPrefix(m.idText, c.prefix) {
				c.mu.Unlock()
				continue
			}
			if p, ok := c.state.pending[m.id]; ok && p.method == "thread/compact/start" && m.failure {
				c.compactionRefused(p.target, p.sent)
			}
			resumed := observationCorrelated && observationPending.intent && observationPending.method == "thread/resume" && observationPending.generation == c.state.Generation
			m = c.state.response(m, raw)
			c.replied(m, raw, observationPending, observationCorrelated, admission, admitted)
			if c.state.Ready && c.owner.owns(c) {
				c.owner.mu.Lock()
				c.owner.reconnectThread = ""
				c.owner.startupBound = true
				c.owner.mu.Unlock()
				c.owner.selected(c.state.Binding)
				warn, warnAt = c.owner.launchHold(c.state.Binding), c.state.Binding
			} else if resumed && c.owner.owns(c) {
				// The terminal's resume selected nothing: the server refused
				// it, or opened the conversation without the right to write.
				c.owner.refusedResume(observationPending.target, resumeRefusal(m, raw))
				warn, warnAt = c.owner.launchHold(c.state.Binding), c.state.Binding
			}
		} else if m.id == "" {
			if m.method == "turn/started" && m.thread == c.state.Thread {
				c.state.fresh = false
			}
			if c.owner.cfg.ToolEvent != nil && toolEvent(m, raw) && m.thread == c.state.Thread && c.owner.owns(c) {
				c.owner.cfg.ToolEvent(raw)
			}
			c.admitted.event(m, raw, time.Now())
			c.recordLateEnds()
			if c.owner.owns(c) && c.state.observing() {
				c.state.events.event(m, raw, time.Now())
			}
			c.announced(m)
			if m.method == "thread/closed" {
				c.state.closedThread(m.thread)
			}
		}
		c.lostSight()
		c.observeServer(m, raw, observationPending, observationCorrelated)
		c.record("server-message", m)
		outcomes := c.collectOutcomes()
		overflow := len(c.state.events.out) > 64 || c.state.events.overflow || c.admitted.overLimit()
		c.mu.Unlock()
		c.complete(outcomes)
		if overflow {
			c.closeWith("server-to-tui", "outcome-capacity", nil, len(raw))
			return
		}
		// A blocked TUI cannot grow a retained transcript queue without limit.
		if !c.queueResponse(raw) {
			c.closeWith("server-to-tui", "response-queue-capacity", nil, len(raw))
			return
		}
		// After the reply that selected the conversation, so the terminal
		// shows the warning in it rather than before it.
		if warn != nil {
			c.warnHold(warnAt, warn)
		}
	}
}

func (c *connection) writeUI() {
	defer c.close()
	for {
		select {
		case <-c.ctx.Done():
			return
		case raw := <-c.toUI:
			err := c.down.writeFrame(1, raw)
			c.responseBytes.Add(-int64(cap(raw)))
			if err != nil {
				c.closeWith("server-to-tui", "write", err, len(raw))
				return
			}
		}
	}
}

func (c *connection) tick() {
	t := time.NewTicker(50 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case now := <-t.C:
			c.mu.Lock()
			c.state.events.expire(now)
			c.admitted.expire(now)
			c.dropStale(now)
			out := c.collectOutcomes()
			overflow := c.admitted.overLimit()
			c.mu.Unlock()
			c.complete(out)
			if overflow {
				c.closeWith("server-to-tui", "outcome-capacity", nil, 0)
				return
			}
		}
	}
}

// resumeRefusal is the server's answer to a resume that selected nothing, as
// the person would read it.
func resumeRefusal(m meta, raw []byte) string {
	if !m.failure && m.directKnown && !m.direct {
		return "the conversation was opened without the right to write to it"
	}
	if !m.failure {
		return "the resume selected no conversation that takes input"
	}
	reason := field(raw, "error", "message")
	if len(reason) > 4096 {
		return "the resume was refused"
	}
	text := decodeText(reason)
	if len(text) > 300 {
		text = text[:300] + "…"
	}
	if text == "" {
		return "the resume was refused"
	}
	return "the resume was refused (" + text + ")"
}
