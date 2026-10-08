package gateway

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"
)

// Display is independent of delivery: this arrival label describes no execution.
// The synthetic item bypasses native observation, admission and outcome paths.
func (c *connection) displayNotice(want Binding, turn, notice string) bool {
	if turn == "" || len(notice) > 64<<10 {
		return false
	}
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return false
	}
	raw, err := json.Marshal(map[string]any{
		"method": "item/completed",
		"params": map[string]any{
			"threadId": want.Thread, "turnId": turn, "completedAtMs": time.Now().UnixMilli(),
			"item": map[string]any{
				"type": "commandExecution", "id": "rewake-notice-display-" + hex.EncodeToString(token),
				"command": "rewake notice --display-only", "cwd": "/", "source": "agent",
				"status": "completed", "commandActions": []any{}, "aggregatedOutput": notice,
				"exitCode": 0, "durationMs": nil, "processId": nil, "pluginId": nil, "scriptPath": nil,
			},
		},
	})
	if err != nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.owner.mu.Lock()
	defer c.owner.mu.Unlock()
	if c.ctx.Err() != nil || c.owner.closed || c.owner.current != c || len(c.owner.owners) != 1 || !sameBinding(want, c.state.Binding) {
		return false
	}
	// Leave native traffic headroom; cosmetic back-pressure never closes transport
	// or changes the outcome of the already acknowledged mailbox delivery.
	if len(c.toUI) >= cap(c.toUI)-4 || c.responseBytes.Load() > transportQueueBytes-(1<<20) {
		return false
	}
	return c.queueResponse(raw)
}
