package cli

import "github.com/iiiokojiadbi/rewake/internal/inbox"

// taskKind is work for another session, and the default. Send returns once the
// session is told; the session's final message comes back later as a
// "Rewake: <session> finished" line.
var taskKind = messageKind{
	kind: inbox.Task,
	wait: defaultWait,
}
