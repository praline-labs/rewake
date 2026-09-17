// Package brief owns text injected into an agent, independent of its transport.
package brief

import (
	"github.com/iiiokojiadbi/rewake/internal/role"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// Context identifies a session and explains its role selection.
type Context struct {
	Name   string
	Room   string
	Role   role.Role
	Reason string
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
