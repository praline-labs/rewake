// Package gateway reserves connection-owned delivery without changing native payloads.
package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

func itoa(v uint64) string         { return strconv.FormatUint(v, 10) }
func decodeText(raw []byte) string { var s string; _ = json.Unmarshal(raw, &s); return s }

type (
	// Config supplies wrapper-owned endpoints and metadata-only callbacks.
	Config struct {
		ReadSequence    func() uint64
		StartupFork     bool
		Upstream, Epoch string
		Record          func(Record)
		Closed          func(CloseInfo)
		Complete        func(Completion)
	}
	// Gateway fences delivery by accepted intent on a single TUI incarnation.
	Gateway struct {
		telemetry         telemetryRun
		startupForkOwner  uint64
		startupForkParent string
		startupBound      bool
		cfg               Config
		workers           sync.WaitGroup
		mu                sync.Mutex
		serial            uint64
		current           *connection
		conns             map[*connection]bool
		owners            map[*connection]bool
		closed            bool
		reconnectThread   string
		gate              chan struct{}
		published         map[string]publishedOutcome
		publishOrder      []string
	}
	connection struct {
		observations                connectionObservations
		requestBytes, responseBytes atomic.Int64
		owner                       *Gateway
		up, down                    *socketClient
		ctx                         context.Context
		cancel                      context.CancelFunc
		once                        sync.Once
		cleaned                     chan struct{}
		mu                          sync.Mutex
		state                       state
		admitted                    admittedWork
		work                        chan func()
		toUI                        chan []byte
		injected                    map[string]chan meta
		next                        uint64
		prefix                      string
	}
)

func (c *connection) close() { c.closeWith("lifecycle", "shutdown", nil, 0) }

func (c *connection) closeWith(direction, reason string, err error, size int) {
	c.once.Do(func() {
		c.cancel()
		_ = c.up.conn.Close()
		_ = c.down.conn.Close()
		c.mu.Lock()
		previous := c.state.Binding
		c.observationClosed()
		c.state.invalidate("connection lost; re-establish recognized primary intent")
		c.state.pending = map[string]pending{}
		c.injected = map[string]chan meta{}
		c.admitted = newAdmittedWork()
		c.mu.Unlock()
		c.owner.release(c, previous)
		if c.owner.cfg.Closed != nil {
			info := CloseInfo{Connection: previous.Connection, Generation: previous.Generation, Direction: direction, Reason: reason, Error: closeError(err), Bytes: size, Requests: len(c.work), Responses: len(c.toUI)}
			var limit *sizeError
			if errors.As(err, &limit) {
				info.SizeStage, info.MessageBytes, info.LimitBytes = limit.Stage, limit.Size, limit.Limit
			}
			c.owner.cfg.Closed(info)
		}
		close(c.cleaned)
	})
}

func (c *connection) schedule() {
	defer c.close()
	for {
		select {
		case <-c.ctx.Done():
			return
		case run := <-c.work:
			run()
		}
	}
}

func (c *connection) readUI() {
	for {
		raw, err := c.down.readMessage()
		if err != nil {
			c.closeWith("tui-to-server", "read", err, 0)
			return
		}
		m, err := project(raw)
		if err != nil {
			c.closeWith("tui-to-server", "projection", err, len(raw))
			return
		}
		// Continue reading approval replies even while a queued lifecycle request
		// waits behind injected admission. Otherwise this can deadlock on the human.
		if m.method == "" {
			if err := c.up.writeFrame(1, raw); err != nil {
				c.closeWith("tui-to-server", "approval-write", err, len(raw))
				return
			}
			continue
		}
		if strings.HasPrefix(m.idText, c.prefix) {
			c.closeWith("tui-to-server", "reserved-id", nil, len(raw))
			return
		}
		if !c.queueRequest(raw, func() {
			if c.owner.acquire(c.ctx) != nil {
				return
			}
			defer func() { <-c.owner.gate }()
			m.reconnect = c.owner.reconnectIntent(c, m)
			m.startupFork = c.owner.startupForkIntent(c, m)
			if recognized(m) || m.startupFork {
				c.owner.claim(c)
			}
			c.mu.Lock()
			m.readClass = c.state.readContext(m)
			var err error
			if isTurnAdmission(m.method) && !isHelper(m) && c.owner.owns(c) && c.state.Ready && m.thread == c.state.Thread {
				err = c.admitted.prepare(m.id, c.state.Binding, false)
				if err == nil {
					c.admitted.capture(c.state.events, m.thread)
				}
			}
			if err == nil {
				err = c.state.request(m)
			}
			if m.method == "thread/compact/start" && err == nil {
				if !c.admitted.manualStart(m.thread, c.state.events) {
					err = errors.New("manual-scope capacity reached; control not forwarded")
				} else {
					c.state.events.manual[m.thread] = true
				}
			}

			c.observeRequest(m)
			c.record("tui-request", m)
			c.mu.Unlock()
			if err == nil {
				c.owner.auxiliaryReadBoundary(c, m)
				err = c.up.writeFrame(1, raw)
			}
			if err != nil {
				c.closeWith("tui-to-server", "request", err, len(raw))
			}
		}) {
			c.closeWith("tui-to-server", "request-queue-capacity", nil, len(raw))
			return
		}
	}
}

func (c *connection) callReserved(ctx context.Context, want Binding, method string, params map[string]any, admissionID ...string) (meta, error) {
	if err := ctx.Err(); err != nil {
		return meta{}, err
	}
	c.mu.Lock()
	now := c.state.Binding
	if !c.owner.owns(c) || !now.Ready || now.Epoch != want.Epoch || now.Connection != want.Connection || now.Generation != want.Generation || now.Thread != want.Thread {
		c.mu.Unlock()
		return meta{}, errors.New("binding unavailable or changed; inspect status and explicitly resume primary")
	}
	var id string
	if len(admissionID) > 0 {
		id = admissionID[0]
		pending, ok := c.admitted.pending["s:"+id]
		if !ok || !sameBinding(pending.binding, now) {
			c.mu.Unlock()
			return meta{}, errors.New("admission reservation expired; nothing sent")
		}
	} else {
		c.next++
		id = c.prefix + itoa(c.next)
		if isTurnAdmission(method) {
			if err := c.admitted.prepare("s:"+id, now, true); err != nil {
				c.mu.Unlock()
				return meta{}, err
			}
			c.admitted.capture(c.state.events, now.Thread)
		}
	}
	wait := make(chan meta, 1)
	c.injected["s:"+id] = wait
	params["threadId"] = now.Thread
	raw, err := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err == nil && endsReadWorkflow(method) {
		c.state.closeReadContext("injected " + method)
	}
	c.record("injected-request", meta{method: method, thread: now.Thread})
	c.mu.Unlock()
	if err != nil {
		return meta{}, err
	}
	if err = c.up.writeFrameContext(ctx, 1, raw); err != nil {
		c.closeWith("injected-to-server", "write", err, len(raw))
		return meta{}, errors.New("delivery transport ended; outcome unknown, do not replay")
	}
	select {
	case reply := <-wait:
		if reply.failure {
			return meta{}, errors.New("native request refused: " + reply.refusal)
		}
		if method == "turn/start" && reply.turn == "" {
			c.closeWith("server-to-injection", "missing-turn-ack", nil, 0)
			return meta{}, errors.New("missing turn acknowledgement; outcome unknown, do not replay")
		}
		return reply, nil
	case <-ctx.Done():
		if method == "thread/read" {
			c.mu.Lock()
			delete(c.injected, "s:"+id)
			c.mu.Unlock()
			return meta{}, ctx.Err()
		}
		c.closeWith("injected-to-server", "admission-timeout", ctx.Err(), 0)
		return meta{}, errors.New("admission timed out; outcome unknown, do not replay")
	case <-c.ctx.Done():
		return meta{}, errors.New("connection ended; outcome unknown, do not replay")
	}
}
