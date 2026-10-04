package gateway

// The gateway's part in the mail tool (docs/mail-bridge-server.md#what-the-wrapper-matches):
// it hands the wrapper the primary thread's turn starts and ends and the
// tool's items, which is how a call is observed natively, and captures that
// thread's ends through the gate the tool's acknowledgments enter.

// toolEvent says whether a server notification concerns the mail tool's
// calls: a turn's start or end, or an MCP tool call's item.
func toolEvent(m meta, raw []byte) bool {
	switch m.method {
	case "turn/started", "turn/completed":
		return true
	case "item/started", "item/completed":
		return str(raw, "params", "item", "type") == "mcpToolCall"
	}
	return false
}

// serverStatus says whether a notification is an MCP server's startup
// status: the tool's server starts with any thread, not only the primary
// one, and its failure is the channel's evidence wherever it came from.
func serverStatus(m meta) bool { return m.method == "mcpServer/startupStatus/updated" }

// primary says whether thread is the one this connection is bound to, while
// it owns the gateway: the only thread whose ends a turn report takes.
func (c *connection) primary(thread string) bool {
	c.mu.Lock()
	bound := c.state.Thread
	c.mu.Unlock()
	return thread != "" && thread == bound && c.owner.owns(c)
}
