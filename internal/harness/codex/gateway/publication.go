package gateway

import "strings"

type publishedOutcome struct{ stopped, final bool }

// Publish only already selected or explicitly retained evidence. It never changes
// routing. IDs from native turns are stable; unidentified gaps need local scope.
func (g *Gateway) publish(v Completion) {
	key := v.Epoch + "/" + v.PublicationID()
	g.mu.Lock()
	old, exists := g.published[key]
	if old.final || v.Kind == "stopped" && old.stopped {
		g.mu.Unlock()
		return
	}
	if v.Kind == "stopped" {
		old.stopped = true
	} else {
		old.final = true
	}
	if !exists {
		g.publishOrder = append(g.publishOrder, key)
	}
	g.published[key] = old
	if len(g.publishOrder) > 256 {
		first := g.publishOrder[0]
		g.publishOrder = g.publishOrder[1:]
		delete(g.published, first)
	}
	emit := g.cfg.Complete
	g.mu.Unlock()
	if emit != nil {
		emit(v)
	}
}

func (c *connection) collectOutcomes() []Completion {
	out := c.admitted.collect()
	if c.state.Ready && c.owner.owns(c) {
		for _, v := range c.state.events.drain() {
			if c.admitted.excluded[v.ID] {
				continue
			}
			if strings.Contains(v.ID, "/gap-") && c.admitted.manual[v.Thread] != nil {
				continue
			}
			v.Epoch = c.state.Epoch
			v.Connection = c.state.Connection
			v.Generation = c.state.Generation
			out = append(out, v)
		}
	}
	return out
}

func (c *connection) complete(out []Completion) {
	for _, v := range out {
		c.owner.publish(v)
	}
}

// PublicationID preserves normal turn identity and scopes run-local gap counters.
func (v Completion) PublicationID() string {
	if strings.Contains(v.ID, "/gap-") {
		return v.ID + "/" + itoa(v.Connection) + "/" + itoa(v.Generation)
	}
	return v.ID
}
