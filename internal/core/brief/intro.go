// Package brief owns text injected into an agent, independent of its transport.
package brief

import (
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// Context identifies a session and explains its role selection.
type Context struct {
	Name   string
	Room   string
	Role   role.Role
	Reason string
	// Harness is what the session's own harness adds: the sentences the
	// role texts need about it — how it commits, say — which the role texts, naming no harness, leave to its adapter
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
