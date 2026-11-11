// Package brief owns text injected into an agent, independent of its transport.
package brief

import (
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/workmode"
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
	// (docs/rules/adapter-api.md#launch). Empty adds nothing.
	Harness role.Section
	// Mode is the runtime work mode whose complete behavioral section follows
	// the role's sections (docs/work-modes.md). Empty Name adds nothing: a
	// harness that switches the mode at runtime injects the section itself,
	// so the launch's briefing never carries a second, stale one.
	Mode workmode.Mode
	// MainImplements and MainPolicyExplicit are the authorship policy this
	// run's launch froze (docs/roles.md#main-implementation): whether main may
	// implement product changes itself, and whether a project file chose that
	// or it is the default. They are said in main's briefing and change its
	// delegation rule; a worker, or a guide outside a session, passes false.
	MainImplements     bool
	MainPolicyExplicit bool
	// Migrations are the obsolete worker settings the launch skipped rather
	// than refused, each with its file and line and what to write instead:
	// the session is told them so that main can repair the configuration it
	// starts under (docs/work-modes.md#the-old-worker-types).
	Migrations []string
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
