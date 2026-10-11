// Package brief owns text injected into an agent, independent of its transport.
package brief

import (
	"github.com/praline-labs/rewake/internal/role"
	"github.com/praline-labs/rewake/internal/state"
)

// Context identifies a session and explains its role selection.
type Context struct {
	Name   string
	Room   string
	Role   role.Role
	Reason string
	// Tool says the run has the mail tool: the briefing then carries the
	// whole transport sentence, and the first clause alone otherwise. No
	// launch sets it yet; S11 derives it from the live tool capability when
	// it builds the briefing.
	Tool bool
	// Harness is what the session's own harness adds: the sentences the
	// role texts need about it — how it commits, how a grant behaves on it —
	// which the role texts, naming no harness, leave to its adapter
	// (docs/v2/design-api.md#launch). Empty adds nothing.
	Harness role.Section
}

// Intro renders the independent system briefing of the selected role.
func Intro(c Context) string {
	if c.Room == "" {
		c.Room = state.DefaultRoom
	}
	c.Role = role.Of(c.Role.ID)
	if c.Reason == "" {
		c.Reason = "requested by the launcher"
	}
	return roleText(c)
}
