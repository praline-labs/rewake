/*
Package role holds what a session is for.

A role is one entry here. The line it adds to the agent's briefing and what the
session owes when its turn ends come from the entry, so a new role is one value
and one line in the list — the way a harness is.
*/
package role

// Role is what a session is for.
type Role struct {
	// ID names the role in a session record and a space. A direct launch is
	// main; workers take their role from the space. General is the reporting
	// fallback for an unknown record.
	ID string
	// Summary describes the role in the guide.
	Summary string
	// Silent says successful turns are reported to nobody. Failures stay visible. It
	// is the exception, so the zero value is a session that reports: a caller
	// that forgets the role must not switch reports off.
	Silent bool
	// Spaces says a session of the role opens spaces and its run hosts them
	// (docs/space.md): main alone, which hands the work out.
	Spaces bool
	// Play is what this role does, in order: the text of the briefing and of
	// the guide's own section. Required — a role without one would launch a
	// session that is told nothing, or, worse, told about another role.
	Play Playbook
}

// General takes work and reports when its turn ends. It is also the fallback role.
var General = Role{
	ID:      "general",
	Summary: "A worker main raises with rewake space open. Takes work from other sessions and reports each turn's end to them.",
	Play:    generalPlaybook,
}

// Main hands out work. It reads every report, so its own turns are reported to
// nobody: reporting back to the sessions that reported to it would never end.
var Main = Role{
	ID:      "main",
	Summary: "The session every rewake <harness> starts. Hands out work and reads the reports; its own successful turns go to nobody, so reports never wake each other endlessly. rewake grants it no permissions beyond what its harness gives.",
	Silent:  true,
	Spaces:  true,
	Play:    mainPlaybook,
}

// Write takes work and can commit changes in its working repository.
var Write = Role{
	ID:      "write",
	Summary: "A worker as general that may commit within its own permissions. The owner's permissions stay as they are.",
	Play:    writePlaybook,
}

// all lists the roles, the default first.
var all = []Role{General, Main, Write}

// All returns the roles, the default first.
func All() []Role { return append([]Role{}, all...) }

// Default is the reporting fallback for old or unknown role records.
func Default() Role { return all[0] }

// Find returns the role with this id.
func Find(id string) (Role, bool) {
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
