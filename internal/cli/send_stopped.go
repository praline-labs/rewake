package cli

import "github.com/iiiokojiadbi/rewake/internal/inbox"

// A stopped turn asks peers to wait, not to send the task again.
var stoppedKind = messageKind{kind: inbox.Stopped, summary: "a turn stopped by the person at the keyboard or by a main with rewake interrupt, or a run that passed unseen, or a turn that ended with no proof of work, as its text says; do not resend or reply."}
