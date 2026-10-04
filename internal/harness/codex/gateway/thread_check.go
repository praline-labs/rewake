package gateway

import "encoding/json"

// refuseThread asks the run's thread check about a terminal request and,
// when it refuses, answers the request itself with the refusal: the server
// never sees it, so no server of that thread starts.
func (c *connection) refuseThread(m meta, raw []byte) bool {
	check := c.owner.cfg.ThreadCheck
	if check == nil {
		return false
	}
	refusal := check(m.method, json.RawMessage(field(raw, "params")))
	if refusal == "" {
		return false
	}
	reply, err := json.Marshal(map[string]any{
		"id":    json.RawMessage(field(raw, "id")),
		"error": map[string]any{"code": -32600, "message": refusal},
	})
	if err != nil {
		return false
	}
	c.mu.Lock()
	c.record("tui-request-refused", m)
	c.mu.Unlock()
	if !c.queueResponse(reply) {
		c.closeWith("server-to-tui", "response-queue-capacity", nil, len(reply))
	}
	return true
}
