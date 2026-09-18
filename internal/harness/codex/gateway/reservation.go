package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Reservation holds the accepted generation through readability, root grants and ACK.
// Native approval replies bypass the gate; the upstream reader never waits for it.
type Reservation struct {
	admissionID string
	sent        bool
	mu          sync.Mutex
	c           *connection
	binding     Binding
	ctx         context.Context
	released    bool
}

var errReadPhase = errors.New("primary resume reads are still in progress")

// Reserve waits outside the admission FIFO and gate for justified read completion.
// A selection change while waiting refuses the old request rather than retargeting it.
func (g *Gateway) Reserve(ctx context.Context) (*Reservation, error) {
	var want Binding
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c := g.currentConnection()
		if c != nil {
			c.mu.Lock()
			b := c.state.Binding
			closed := c.state.deliverySettled()
			c.mu.Unlock()
			if want.Ready && !sameBinding(want, b) {
				return nil, errors.New("conversation changed while waiting for delivery admission")
			}
			if b.Ready && g.owns(c) {
				want = b
				if closed {
					r, err := c.reserve(ctx, want)
					if !errors.Is(err, errReadPhase) {
						return r, err
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%w: selected conversation is not ready; wait for native resume to finish or select /resume or /new", ctx.Err())
		case <-ticker.C:
		}
	}
}

func sameBinding(a, b Binding) bool {
	return a.Epoch == b.Epoch && a.Connection == b.Connection && a.Generation == b.Generation && a.Thread == b.Thread && b.Ready
}

type reserveResult struct {
	reservation *Reservation
	err         error
}

func (c *connection) reserve(ctx context.Context, want Binding) (*Reservation, error) {
	answer := make(chan reserveResult, 1)
	select {
	case c.work <- func() {
		if err := c.owner.acquire(ctx); err != nil {
			answer <- reserveResult{err: err}
			return
		}
		c.mu.Lock()
		valid := sameBinding(want, c.state.Binding) && c.owner.owns(c)
		closed := c.state.deliverySettled()
		c.mu.Unlock()
		if !valid || !closed {
			<-c.owner.gate
			err := errReadPhase
			if !valid {
				err = errors.New("delivery binding changed before reservation")
			}
			answer <- reserveResult{err: err}
			return
		}
		c.mu.Lock()
		c.next++
		id := c.prefix + itoa(c.next)
		err := c.admitted.prepare("s:"+id, want, true)
		if err == nil {
			c.admitted.capture(c.state.events, want.Thread)
		}
		c.mu.Unlock()
		if err != nil {
			<-c.owner.gate
			answer <- reserveResult{err: err}
			return
		}
		r := &Reservation{c: c, binding: want, ctx: ctx, admissionID: id}
		context.AfterFunc(ctx, r.Close)
		answer <- reserveResult{reservation: r}
	}:
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: selected conversation is not ready; wait for native resume to finish or select /resume or /new", ctx.Err())
	case <-c.ctx.Done():
		return nil, errors.New("connection ended before reservation")
	default:
		return nil, errors.New("admission queue full; nothing sent")
	}
	select {
	case result := <-answer:
		return result.reservation, result.err
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: selected conversation is not ready; wait for native resume to finish or select /resume or /new", ctx.Err())
	case <-c.ctx.Done():
		return nil, errors.New("connection ended before reservation")
	}
}

func (r *Reservation) valid() error {
	if r.released {
		return errors.New("delivery reservation released")
	}
	if err := r.ctx.Err(); err != nil {
		return err
	}
	r.c.mu.Lock()
	defer r.c.mu.Unlock()
	if !sameBinding(r.binding, r.c.state.Binding) || !r.c.owner.owns(r.c) {
		return errors.New("delivery reservation lost its accepted conversation")
	}
	return nil
}

// Prepare keeps expiry from releasing the fence midway through making mail readable.
func (r *Reservation) Prepare(fn func(string) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.valid(); err != nil {
		return err
	}
	return fn(r.binding.Thread)
}

// Close releases admission even when mailbox publication or Git metadata lookup fails.
func (r *Reservation) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.released {
		r.released = true
		r.c.mu.Lock()
		delete(r.c.admitted.pending, "s:"+r.admissionID)
		r.c.mu.Unlock()
		<-r.c.owner.gate
	}
}

// ReadThread requests only current environment metadata, never stored conversation turns.
func (r *Reservation) ReadThread(ctx context.Context, result any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.valid(); err != nil {
		return err
	}
	reply, err := r.c.callReserved(ctx, r.binding, "thread/read", map[string]any{"includeTurns": false})
	if err != nil {
		return err
	}
	raw, err := json.Marshal(map[string]any{"thread": map[string]any{"id": reply.thread, "status": map[string]string{"type": reply.status}, "environments": reply.environments}})
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, result)
}

// Deliver sends once under the same generation and additive workspace snapshot.
func (r *Reservation) Deliver(ctx context.Context, messageID, notice string, roots []string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.valid(); err != nil {
		return "", err
	}
	if len(notice) > 64<<10 || messageID == "" {
		return "", errors.New("invalid notice")
	}
	if r.sent {
		return "", errors.New("delivery reservation already used; do not replay")
	}
	r.sent = true
	params := map[string]any{"clientUserMessageId": messageID, "input": []any{map[string]any{"type": "text", "text": notice}}}
	if roots != nil {
		params["runtimeWorkspaceRoots"] = roots
	}
	reply, err := r.c.callReserved(ctx, r.binding, "turn/start", params, r.admissionID)
	return reply.turn, err
}

// A native policy update may await approval or an asynchronous server response.
// Observe its ACK before reading replacement roots; do not hold admission while waiting.
func (s *state) deliverySettled() bool {
	if s.readPhase() != "closed" {
		return false
	}
	for _, p := range s.pending {
		if p.method == "thread/settings/update" && p.target == s.Thread {
			return false
		}
	}
	return true
}
