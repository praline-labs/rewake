package gateway

import (
	"context"
	"net/http"
	"time"
)

// New waits for a recognized intent, not merely the first TCP/Unix connection.
func New(cfg Config) *Gateway {
	return &Gateway{cfg: cfg, conns: map[*connection]bool{}, owners: map[*connection]bool{}, gate: make(chan struct{}, 1), published: map[string]publishedOutcome{}, proofs: newProofs(), ops: newOperations()}
}

// Close releases every helper and intent-bearing connection with the wrapper.
func (g *Gateway) Close() {
	g.mu.Lock()
	g.closed = true
	var all []*connection
	for c := range g.conns {
		all = append(all, c)
	}
	g.mu.Unlock()
	for _, c := range all {
		c.close()
	}
	g.workers.Wait()
}

func (g *Gateway) currentConnection() *connection { g.mu.Lock(); defer g.mu.Unlock(); return g.current }

func (g *Gateway) owns(c *connection) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return c.ctx.Err() == nil && !g.closed && g.current == c && len(g.owners) == 1
}

func (g *Gateway) claim(c *connection) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed || !g.conns[c] {
		return
	}
	g.owners[c] = true
	if len(g.owners) == 1 {
		g.current = c
	} else {
		g.current = nil
		g.reconnectThread = ""
		g.telemetry.mu.Lock()
		g.telemetry.partial = true
		g.telemetry.mu.Unlock()
	}
}

func (g *Gateway) release(c *connection, previous Binding) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.current == c && len(g.owners) == 1 && previous.Ready {
		g.reconnectThread = previous.Thread
	}
	delete(g.conns, c)
	delete(g.owners, c)
	if g.current == c {
		g.current = nil
	}
}

// Binding never promotes an auxiliary connection or revives a prior conflicted one.
func (g *Gateway) Binding() Binding {
	c := g.currentConnection()
	if c == nil {
		return Binding{Epoch: g.cfg.Epoch, Reason: "no unique accepted-intent connection; select a conversation with /resume or /new"}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	b := c.state.Binding
	if !g.owns(c) {
		b.Ready = false
		b.Thread = ""
		b.Reason = "primary connection changed or conflicted; select a conversation with /resume or /new"
	}
	return b
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	down, err := upgrade(w, r)
	if err != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	dc, stop := context.WithTimeout(ctx, 3*time.Second)
	up, err := dialSocket(dc, g.cfg.Upstream)
	stop()
	if err != nil {
		cancel()
		_ = down.conn.Close()
		return
	}
	g.mu.Lock()
	if g.closed || len(g.conns) >= 16 {
		g.mu.Unlock()
		cancel()
		_ = up.conn.Close()
		_ = down.conn.Close()
		return
	}
	g.serial++
	c := &connection{cleaned: make(chan struct{}), owner: g, up: up, down: down, ctx: ctx, cancel: cancel, work: make(chan func(), transportQueueItems), toUI: make(chan []byte, transportQueueItems), state: newState(g.cfg.Epoch, g.serial), admitted: newAdmittedWork(), injected: map[string]chan meta{}, prefix: "rewake-inject-" + g.cfg.Epoch + "-" + itoa(g.serial) + "-"}
	c.admitted.proven = g.proofs
	c.state.ops = g.ops
	g.conns[c] = true
	g.workers.Add(2)
	g.mu.Unlock()
	defer c.close()
	go c.schedule()
	go func() { defer g.workers.Done(); c.readServer() }()
	go c.writeUI()
	go func() { defer g.workers.Done(); c.tick() }()
	c.readUI()
}

func (g *Gateway) acquire(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case g.gate <- struct{}{}:
	}
	if err := ctx.Err(); err != nil {
		<-g.gate
		return err
	}
	return nil
}

// The native reconnect path explicitly resumes its selected thread without root
// overrides. Correlate that request with the last unambiguous closed owner; an
// anchor alone never selects, subscribes, sends work or restores a result ledger.
func (g *Gateway) reconnectIntent(c *connection, m meta) bool {
	if m.method != "thread/resume" || !m.numeric || m.thread == "" || m.roots {
		return false
	}
	c.mu.Lock()
	fresh := c.state.initialized && c.state.Generation == 0
	c.mu.Unlock()
	if !fresh {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return !g.closed && g.current == nil && len(g.owners) == 0 && g.reconnectThread == m.thread
}
