/*
Package role holds what a session is for.

A role is one entry here. The launch flag that selects it, the line it adds to
the agent's briefing and what the session owes when its turn ends all come from
the entry, so a new role is one value and one line in the list — the way a
harness is.
*/
package role

// Role is what a session is for.
type Role struct {
	// ID names the role in a session record and, as --<id>, selects it at
	// launch. Omitting a role flag always uses general.
	ID string
	// Summary is the help line of the launch flag.
	Summary string
	// Silent says successful turns are reported to nobody. Failures stay visible. It
	// is the exception, so the zero value is a session that reports: a caller
	// that forgets the role must not switch reports off.
	Silent bool
	// GitWrite permits receiving an explicit main-authorized metadata grant.
	// Eligibility alone never changes permissions at launch or message delivery.
	GitWrite bool
	// Play is what this role does, in order: the text of the briefing and of
	// the guide's own section. Required — a role without one would launch a
	// session that is told nothing, or, worse, told about another role.
	Play Playbook
}

// General takes work and reports when its turn ends. It is also the fallback role.
var General = Role{
	ID:      "general",
	Summary: "The default. Takes work from other sessions and reports each turn's end to them; may start before main.",
	Play:    generalPlaybook,
}

// Main hands out work. It reads every report, so its own turns are reported to
// nobody: reporting back to the sessions that reported to it would never end.
var Main = Role{
	ID:       "main",
	Summary:  "Hands out work and reads the reports; its own successful turns go to nobody, so reports never wake each other endlessly. Alone may grant a worker a directory or Git metadata with a task; rewake gives it no write permission of its own.",
	Silent:   true,
	GitWrite: true,
	Play:     mainPlaybook,
}

// Write takes work and can commit changes in its working repository.
var Write = Role{
	ID:       "write",
	Summary:  "As general, and may commit: on a harness that takes a Git grant once main's --grant-git task opens the Git metadata, on Claude Code within its own permissions. The owner's permissions stay as they are.",
	GitWrite: true,
	Play:     writePlaybook,
}

// all lists the roles, the default first.
var all = []Role{General, Main, Write}

// All returns the roles, the default first.
func All() []Role { return append([]Role{}, all...) }

// Default is the reporting fallback for old or unknown role records.
func Default() Role { return all[0] }

// Find returns the role with this id. An empty id is the default: records
// written before roles existed have none.
func Find(id string) (Role, bool) {
	// legacy(rewake <2026-09-17): records of earlier builds carry no role or the old id worker; remove when no session started by such a build is registered
	if id == "" || id == "worker" {
		return Default(), true
	}
	for _, candidate := range all {
		if candidate.ID == id {
			return candidate, true
		}
	}
	return Role{}, false
}

// Of returns the role of a session record's role id, taking an unknown one for
// the default: a record from a newer rewake should not stop an older one.
func Of(id string) Role {
	found, ok := Find(id)
	if !ok {
		return Default()
	}
	return found
}
