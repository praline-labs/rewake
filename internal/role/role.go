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
	// launch. The default role is chosen by giving no role flag at all.
	ID string
	// Summary is the help line of the launch flag.
	Summary string
	// Silent says the end of this session's turns is reported to nobody. It
	// is the exception, so the zero value is a session that reports: a caller
	// that forgets the role must not switch reports off.
	Silent bool
	// GitWrite permits a launch to grant writes to the working repository metadata.
	// It is separate from reporting: a writer reports, while main stays silent.
	GitWrite bool
	// Brief is what the agent is told about its part, after who it is.
	Brief string
}

// Worker takes work and reports when its turn ends. It is the default.
var Worker = Role{
	ID:      "worker",
	Summary: "Takes work from other sessions and reports the end of each turn to them. The default.",
	Brief:   "When you finish work another session gave you, end your turn with the result as your final message and stop: rewake delivers that message to it.",
}

// Main hands out work. It reads every report, so its own turns are reported to
// nobody: reporting back to the sessions that reported to it would never end.
var Main = Role{
	ID:       "main",
	Summary:  "The session that hands out work: it gets reports, reports no turns, and requests permission to commit in the working repository.",
	Silent:   true,
	GitWrite: true,
	Brief:    "You are the main session: sessions you give work to report back to you when their turn ends, and your own turns are reported to nobody.",
}

// Write takes work and can commit changes in its working repository.
var Write = Role{
	ID:       "write",
	Summary:  "Takes work and reports its turns, with permission to commit in the working repository.",
	GitWrite: true,
	Brief:    Worker.Brief + " You can commit changes in the repository of your working directory.",
}

// all lists the roles, the default first.
var all = []Role{Worker, Main, Write}

// All returns the roles, the default first.
func All() []Role { return append([]Role{}, all...) }

// Default is the role of a session started without a role flag.
func Default() Role { return all[0] }

// Find returns the role with this id. An empty id is the default: records
// written before roles existed have none.
func Find(id string) (Role, bool) {
	if id == "" {
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
