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
	// whole transport sentence, and the first clause alone otherwise. A run
	// that builds its briefing after its readiness exchange sets it from the
	// live tool capability (the fixture's); one whose briefing goes on the
	// harness's command line has proven nothing yet and leaves it false.
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
