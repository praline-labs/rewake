package cli

import "github.com/iiiokojiadbi/rewake/internal/inbox"

// A keyboard stop asks peers to wait, not to send the task again.
var stoppedKind = messageKind{kind: inbox.Stopped, summary: "a keyboard interruption; wait for the person, do not resend or reply."}
