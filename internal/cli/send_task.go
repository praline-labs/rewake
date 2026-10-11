package cli

import "github.com/praline-labs/rewake/internal/core/mail"

// taskKind is work for another session, and the default. Send returns once the
// session is told; the session's final message comes back later as a
// "Rewake: <session> finished" line.
var taskKind = messageKind{
	kind:    mail.Task,
	summary: "work, reported when the turn ends.",
	wait:    defaultWait,
}
