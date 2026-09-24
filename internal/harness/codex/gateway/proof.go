package gateway

// A turn is published only with proof that it is work: a successful reply to
// turn/start, turn/steer or review/start named it, or it had an item other than
// contextCompaction. A compaction's turn gives neither, whoever asked for it,
// so no compaction settles a task, whatever the marks say and on whichever
// connection it ends. A turn without proof, or a gap, a run whose turn is
// unknown, is published only as advisory, which settles nothing; a turn shown
// by its item to be a compaction is not published at all
// (docs/remote-control-codex.md).

import (
	"strings"
	"sync"
	"time"
)

// proofs names the turns shown to be work. The gateway keeps one for all its
// connections: a turn named on one may end on the next, after the terminal
// reconnects.
type proofs struct {
	mu    sync.Mutex
	work  map[string]bool
	order []string
	// compacting names the turns that had a contextCompaction item.
	compacting   map[string]bool
	compactOrder []string
}

func newProofs() *proofs { return &proofs{work: map[string]bool{}, compacting: map[string]bool{}} }

func (p *proofs) add(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.work[id] {
		return
	}
	p.work[id] = true
	p.order = append(p.order, id)
	if len(p.order) > 1024 {
		delete(p.work, p.order[0])
		p.order = p.order[1:]
	}
}

// compaction records a turn that had a contextCompaction item. Only a turn
// not shown to be work is taken for a compaction by it: an ordinary turn
// compacts inside itself with the same item.
func (p *proofs) compaction(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.compacting[id] {
		return
	}
	p.compacting[id] = true
	p.compactOrder = append(p.compactOrder, id)
	if len(p.compactOrder) > 1024 {
		delete(p.compacting, p.compactOrder[0])
		p.compactOrder = p.compactOrder[1:]
	}
}

// compacted says id had a contextCompaction item and no proof of work.
func (p *proofs) compacted(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.compacting[id] && !p.work[id]
}

func (p *proofs) has(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.work[id]
}

// shown says v may be published as it is: its turn is known to be work, or
// it is a gap's, which is advisory and settles nothing (observer.expire).
func (c *connection) shown(v Completion) bool {
	return strings.Contains(v.ID, "/gap-") || c.admitted.proven.has(v.ID)
}

// proofHoldLimit bounds how long an outcome waits for its proof before it is
// reported as advisory. A reply travels apart from its turn's events and may
// come after its turn/completed, by moments. Gateway.proofHold changes it for
// the tests.
const proofHoldLimit = 500 * time.Millisecond

func (g *Gateway) proofLimit() time.Duration {
	if g != nil && g.proofHold > 0 {
		return g.proofHold
	}
	return proofHoldLimit
}

// advisoryText is the advisory outcome of a turn that ended with no proof of
// work: a goal's that failed before its first item, or one whose reply was
// lost with the connection. What is known of its end follows it.
const advisoryText = "a turn of this conversation ended without rewake seeing what it did"

// unprovenWork is an outcome waiting for its proof, since it was read; advised
// once its advisory has gone.
type unprovenWork struct {
	v       Completion
	since   time.Time
	advised bool
}

// advisory is the report of v with no proof of work: a stopped outcome that
// settles nothing, and none for a compaction's turn, which is no outcome of
// anyone's task. It carries no start or end, so it takes no pending mark, and
// an identity of its own, so the turn's own outcome after a late proof — a
// stopped one too — is neither dropped as its duplicate here nor merged with it
// in the turn receipts.
func (c *connection) advisory(v Completion) (Completion, bool) {
	if c.admitted.proven.compacted(v.ID) {
		return Completion{}, false
	}
	a := v
	a.ID, a.Kind, a.Text, a.Started, a.Ended = v.ID+"/advisory", "stopped", advisoryText, 0, 0
	if v.Kind != "finished" && v.Text != "" {
		a.Text += "; " + v.Text
	}
	return a, true
}

// proven passes on the outcomes shown to be work, and keeps the others for a
// proof that comes later. One kept past proofLimit is reported as advisory
// and still kept, up to 16, so a late proof publishes its own outcome; one
// pushed out, or left when the connection ends (abandon), is reported too
// if it has not been. Called under c.mu.
func (c *connection) proven(out []Completion, now time.Time) []Completion {
	var ready []Completion
	waiting := c.unproven[:0:0]
	for _, v := range out {
		c.unproven = append(c.unproven, unprovenWork{v: v, since: now})
	}
	for _, w := range c.unproven {
		switch {
		case c.shown(w.v):
			ready = append(ready, w.v)
			continue
		case !w.advised && now.Sub(w.since) >= c.owner.proofLimit():
			ready = c.advise(ready, &w)
		}
		waiting = append(waiting, w)
	}
	for len(waiting) > 16 {
		ready = c.advise(ready, &waiting[0])
		waiting = waiting[1:]
	}
	c.unproven = waiting
	return ready
}

func (c *connection) advise(ready []Completion, w *unprovenWork) []Completion {
	if w.advised {
		return ready
	}
	w.advised = true
	if a, ok := c.advisory(w.v); ok {
		ready = append(ready, a)
	}
	return ready
}

// abandon reports every outcome still waiting when the connection ends: its
// proof, a reply, is lost with it. Called under c.mu.
func (c *connection) abandon() []Completion {
	var ready []Completion
	for i := range c.unproven {
		ready = c.advise(ready, &c.unproven[i])
	}
	c.unproven = nil
	return ready
}
