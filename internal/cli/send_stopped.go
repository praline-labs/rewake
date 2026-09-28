package cli

import "github.com/praline-labs/rewake/internal/inbox"

// A stopped turn asks peers to wait, not to send the task again.
var stoppedKind = messageKind{kind: inbox.Stopped, summary: "a turn stopped at the keyboard or by a main's rewake interrupt, a run that passed unseen, or a turn that ended with no proof of work, as its text says; do not resend."}
