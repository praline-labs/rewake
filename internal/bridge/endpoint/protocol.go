/*
Package endpoint is the wrapper's context endpoint of the mail tool
(docs/archive-1.x/mail-bridge-server.md#who-calls): the one place that knows which native
call a tool call is. The harness's own events, through the neutral input, record
what the model called; the server asks for a ticket
naming one of those calls; the child running the call has the ticket confirmed
once; and the result the harness recorded acknowledges a read. The endpoint
lives in the wrapper's memory and goes with it, so a ticket means nothing to
another run.

The same package holds the clients: the server's, the child's and the
transport's side of each exchange.
*/
package endpoint

import (
	"github.com/praline-labs/rewake/internal/bridge"
)

// The roles a connection says hello as.
const (
	roleServer = "server"
	roleChild  = "child"
	// roleTransport is a harness's own side of a tool call: the process
	// the wrapper started, asking for one call to run (transport.go).
	roleTransport = "transport"
)

// The operations each role may ask.
const (
	opTicket  = "ticket"
	opConfirm = "confirm"
	opCall    = "call"
)

// hello is the first line of every connection.
type hello struct {
	Role string `json:"role"`
	// Capability is the per-launch secret; a transport, the harness's own
	// process, carries none.
	Capability string `json:"capability,omitempty"`
}

// request is one line after the hello. Requests on one connection are
// answered as each is done, matched by ID.
type request struct {
	ID     uint64         `json:"id"`
	Op     string         `json:"op"`
	Ask    *TicketRequest `json:"ask,omitempty"`
	Ticket *bridge.Ticket `json:"ticket,omitempty"`
	// Call is a transport's call to run.
	Call *ToolCall `json:"call,omitempty"`
}

// response answers a hello (ID zero) or a request.
type response struct {
	ID     uint64         `json:"id"`
	Error  string         `json:"error,omitempty"`
	Ticket *bridge.Ticket `json:"ticket,omitempty"`
	Answer *ToolAnswer    `json:"answer,omitempty"`
}

// TicketRequest is what the server knows of a call when it asks for a
// ticket: the ids the harness put in the call's _meta, and the words.
type TicketRequest struct {
	Transport    string `json:"transport"`
	Conversation string `json:"conversation,omitempty"`
	Turn         string `json:"turn,omitempty"`
	CallID       string `json:"callId"`
	// TurnsNeverReused is the transport's word that a turn id it binds a call
	// to is never given to another turn: a later attempt bound to the same
	// conversation and turn then runs in that turn
	// (docs/mail-bridge-turns.md#a-pending-mark-at-its-turns-end).
	TurnsNeverReused bool `json:"turnsNeverReused,omitempty"`
	// Words are the normalized words and Digest their digest.
	Words  []string `json:"words"`
	Digest string   `json:"digest"`
	// Arrived is when the call reached the server, on the boot clock.
	Arrived int64 `json:"arrived"`
}

// maxLine bounds one line either way: a transport's call and its answer
// carry the words and the whole result this tool returns.
const maxLine = 1 << 20
