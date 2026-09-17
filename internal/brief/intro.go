// Package brief owns text injected into an agent, independent of its transport.
package brief

import (
	"fmt"
	"strings"

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

// Intro gives the agent its room and selected role before pointing it to the
// guide. The reason matters when a launch without a flag becomes main.
func Intro(request Context) string {
	part := request.Role
	if part.ID == "" {
		part = role.Default()
	}
	room := request.Room
	if room == "" {
		room = state.DefaultRoom
	}
	reason := request.Reason
	if reason == "" {
		reason = "requested by the launcher"
	}
	return strings.Join([]string{
		fmt.Sprintf("You are running inside rewake as session %q in room %q.", request.Name, room),
		fmt.Sprintf("Your role is %s: %s.", part.ID, reason),
		"Only sessions in this room can see and message each other.",
		"rewake lets agent sessions on this machine message each other; a message waiting for you is announced by a line with \"Rewake: <session> <kind>\".",
		"Run `rewake guide` before you send or read messages: it explains how.",
		roleText(part.ID),
	}, "\n")
}
