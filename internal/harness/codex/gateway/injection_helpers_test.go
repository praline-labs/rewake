package gateway

import (
	"context"
	"errors"
	"time"
)

// Deliver bypasses readiness waits only in regression tests that force historical
// overlap. Production injection is available solely through Reservation.
func (g *Gateway) Deliver(ctx context.Context, want Binding, messageID, notice string) (string, error) {
	if len(notice) > 64<<10 || messageID == "" {
		return "", errors.New("invalid notice")
	}
	return g.callBound(ctx, want, "turn/start", map[string]any{"clientUserMessageId": messageID, "input": []any{map[string]any{"type": "text", "text": notice}}})
}

// Materialize persists an empty fixture thread without model input.
func (g *Gateway) Materialize(ctx context.Context, want Binding) error {
	_, e := g.callBound(ctx, want, "thread/name/set", map[string]any{"name": "gateway-fixture-" + itoa(want.Generation)})
	return e
}

func (g *Gateway) callBound(ctx context.Context, want Binding, method string, params map[string]any) (string, error) {
	c := g.currentConnection()
	if c == nil {
		return "", errors.New("no TUI connection")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	result := make(chan callResult, 1)
	select {
	case c.work <- func() { turn, err := c.callAdmitted(ctx, want, method, params); result <- callResult{turn, err} }:
	case <-ctx.Done():
		return "", ctx.Err()
	case <-c.ctx.Done():
		return "", errors.New("connection ended")
	default:
		return "", errors.New("admission queue full; nothing sent")
	}
	select {
	case v := <-result:
		return v.turn, v.err
	case <-ctx.Done():
		c.close()
		return "", errors.New("admission deadline; outcome unknown unless canceled before sending, do not replay")
	case <-c.ctx.Done():
		return "", errors.New("connection ended; outcome unknown, do not replay")
	}
}

type callResult struct {
	turn string
	err  error
}

func (c *connection) callAdmitted(ctx context.Context, want Binding, method string, params map[string]any) (string, error) {
	if err := c.owner.acquire(ctx); err != nil {
		return "", err
	}
	defer func() { <-c.owner.gate }()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	reply, err := c.callReserved(ctx, want, method, params)
	return reply.turn, err
}
